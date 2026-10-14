// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/require"
)

func importTestImportContext(t *testing.T) (*ImportContext, []ImportBinding, *tfe.Workspace) {
	t.Helper()
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	identity := map[string]any{"bucket": "sample"}
	marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", identity)
	require.NoError(t, err)
	importContext := &ImportContext{QueryRunID: "qry-example", Providers: map[string]queryResultProvider{"aws_s3_buckets": provider}, Target: ImportContextTarget{WorkspaceID: "ws-example", OrganizationName: "org", IdentitySupport: map[string]string{"aws_s3_bucket": "unknown"}}, LockedProviders: map[string]string{provider.Source: "6.66.0"}, SelectedResults: []ImportContextSelection{{QueryResultMarker: marker, ListType: "aws_s3_buckets", ManagedType: "aws_s3_bucket", Identity: identity, SearchIdentityVersion: 0}}}
	importContext.SelectionDigest, err = importDigest(*importContext)
	require.NoError(t, err)
	return importContext, []ImportBinding{{QueryResultMarker: marker, TargetAddress: "aws_s3_bucket.sample"}}, &tfe.Workspace{ID: "ws-example", Organization: &tfe.Organization{Name: "org"}}
}

func TestPrepareImportInputsRequireExactLockAndExplicitSelection(t *testing.T) {
	input := PrepareImportInput{OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example", LockedProviders: map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"}}
	importContext, _, _ := importTestImportContext(t)
	input.Selections = []ImportSelection{{QueryResultMarker: importContext.SelectedResults[0].QueryResultMarker, ManagedType: "aws_s3_bucket"}}
	require.True(t, validImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = ">= 6.0"
	require.False(t, validImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = "6.66.0"
	input.Selections = append(input.Selections, input.Selections[0])
	require.False(t, validImportInput(input))
}

func TestVerifyImportContextAndBindingIntegrity(t *testing.T) {
	importContext, bindings, workspace := importTestImportContext(t)
	selected, addresses, err := validateImportContext(importContext, bindings, workspace)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, "aws_s3_bucket.sample", addresses[importContext.SelectedResults[0].QueryResultMarker])
	importContext.LockedProviders["registry.terraform.io/hashicorp/aws"] = "6.66.1"
	_, _, err = validateImportContext(importContext, bindings, workspace)
	require.EqualError(t, err, "import_context_digest_mismatch")
	importContext.SelectionDigest, err = importDigest(*importContext)
	require.NoError(t, err)
	bindings[0].TargetAddress = ""
	_, _, err = validateImportContext(importContext, bindings, workspace)
	require.EqualError(t, err, "binding_invalid")
	importContext.SelectedResults[0].QueryResultMarker = "candidate-v0"
	importContext.SelectionDigest, err = importDigest(*importContext)
	require.NoError(t, err)
	_, _, err = validateImportContext(importContext, bindings, workspace)
	require.EqualError(t, err, "import_context_marker_invalid")
}

func TestVerifyIdentityNeverUsesImportIDAsIdentity(t *testing.T) {
	importContext, _, _ := importTestImportContext(t)
	selection := importContext.SelectedResults[0]
	after := json.RawMessage(`{"bucket":"sample"}`)
	status, _ := compareImportPlanIdentity(selection, map[string]uint64{"aws_s3_bucket.sample": 0}, "aws_s3_bucket.sample", after, "1.12.0", "unknown")
	require.Equal(t, "matched", status)
	status, reason := compareImportPlanIdentity(selection, nil, "aws_s3_bucket.sample", after, "1.12.0", "unknown")
	require.Equal(t, "unverified", status)
	require.Equal(t, "identity_schema_version_not_compared", reason)
	status, reason = compareImportPlanIdentity(selection, map[string]uint64{"aws_s3_bucket.sample": 1}, "aws_s3_bucket.sample", after, "1.12.0", "unknown")
	require.Equal(t, "unverified", status)
	require.Equal(t, "identity_schema_version_differs", reason)
	status, _ = compareImportPlanIdentity(selection, map[string]uint64{"aws_s3_bucket.sample": 0}, "aws_s3_bucket.sample", json.RawMessage(`{"bucket":"other"}`), "1.12.0", "unknown")
	require.Equal(t, "mismatched", status)
	status, _ = compareImportPlanIdentity(selection, map[string]uint64{"aws_s3_bucket.sample": 0}, "aws_s3_bucket.sample", nil, "1.12.0", "unknown")
	require.Equal(t, "unverified", status)
}

func TestImportResultLimitsIncludeBothCopies(t *testing.T) {
	result := PrepareImportResult{Status: "prepared", NextAction: string(make([]byte, maxImportResultBytes/2))}
	require.EqualError(t, checkImportResultBudget(result), "import_response_limit")
}

func TestVerifyImportRejectsDuplicateDetailAddresses(t *testing.T) {
	_, err := ReadVerifyImportPlan(context.Background(), nil, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", DetailAddresses: []string{"aws_s3_bucket.sample", "aws_s3_bucket.sample"}})
	require.EqualError(t, err, "verify_input_invalid")
}

func TestVerifyImportRunGuardsPlanJSONRead(t *testing.T) {
	for _, tc := range []struct {
		name, workspace, planOnly, autoApply, want string
	}{
		{"wrong workspace", "ws-other", "true", "false", "run_workspace_mismatch"},
		{"missing plan-only", "ws-example", "null", "false", "run_not_plan_only"},
		{"missing auto-apply", "ws-example", "true", "null", "run_not_plan_only"},
		{"auto-apply enabled", "ws-example", "true", "true", "run_not_plan_only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var planReads int
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/organizations/org/workspaces/target":
					_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-example","attributes":{"name":"target"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
				case "/api/v2/runs/run-example":
					_, _ = fmt.Fprintf(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":"planned_and_finished","plan-only":%s,"auto-apply":%s},"relationships":{"workspace":{"data":{"type":"workspaces","id":%q}},"plan":{"data":{"type":"plans","id":"plan-example"}},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-example"}}}}}`, tc.planOnly, tc.autoApply, tc.workspace)
				default:
					planReads++
					http.NotFound(w, r)
				}
			}))
			defer backend.Close()
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
			c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
			require.NoError(t, err)
			out, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"})
			require.NoError(t, err)
			require.Equal(t, "blocked", out.Status)
			require.Equal(t, []string{tc.want}, out.Diagnostics)
			require.Zero(t, planReads, "no Plan metadata or JSON read is allowed")
		})
	}
}
