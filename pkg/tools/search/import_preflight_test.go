// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	require.True(t, validNormalizedPrepareImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = ">= 6.0"
	require.False(t, validNormalizedPrepareImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = "6.66.0"
	input.Selections = append(input.Selections, input.Selections[0])
	require.False(t, validNormalizedPrepareImportInput(input))
}

func TestPrepareImportPreservesTerraformDomainValidation(t *testing.T) {
	validMarker := "qrm1-" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name           string
		providerSource string
		marker         string
		managedType    string
	}{
		{name: "invalid provider source", providerSource: "bad source/aws", marker: validMarker, managedType: "aws_s3_bucket"},
		{name: "invalid query result marker", providerSource: "registry.terraform.io/hashicorp/aws", marker: "opaque-result", managedType: "aws_s3_bucket"},
		{name: "invalid managed type", providerSource: "registry.terraform.io/hashicorp/aws", marker: validMarker, managedType: "aws-s3-bucket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := PrepareImportInput{
				OrganizationName: "org",
				WorkspaceName:    "target",
				QueryRunID:       "opaque query run id",
				Selections:       []ImportSelection{{QueryResultMarker: tc.marker, ManagedType: tc.managedType}},
				LockedProviders:  map[string]string{tc.providerSource: "6.66.0"},
			}
			require.False(t, validNormalizedPrepareImportInput(input))
		})
	}

	_, err := validateNormalizedVerifyImportRequest(VerifyImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		RunID:            "opaque run id",
		DetailAddresses:  []string{"aws_s3_bucket.invalid?address"},
	}, defaultVerifyImportPollConfig())
	require.EqualError(t, err, "verify_input_invalid")
}

func TestPrepareImportProviderSourceDiagnostics(t *testing.T) {
	validMarker := "qrm1-" + strings.Repeat("a", 64)
	input := PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections: []ImportSelection{{QueryResultMarker: validMarker, ManagedType: "aws_s3_bucket"}},
	}

	input.LockedProviders = map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input))
	input.LockedProviders = map[string]string{"providers.example.com/team/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input), "custom registry hosts remain valid")
	canonical, err := canonicalizeLockedProviders(input.LockedProviders)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"providers.example.com/team/aws": "6.18.0"}, canonical, "custom registry host is preserved")

	input.LockedProviders = map[string]string{"hashicorp/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input), "public registry shorthand is canonicalizable")
	canonical, err = canonicalizeLockedProviders(input.LockedProviders)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}, canonical)

	input.LockedProviders = map[string]string{"datadog/datadog": "3.0.0"}
	canonical, err = canonicalizeLockedProviders(input.LockedProviders)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"registry.terraform.io/datadog/datadog": "3.0.0"}, canonical, "public shorthand is not limited to the hashicorp namespace")

	input.LockedProviders = map[string]string{"aws": "6.18.0"}
	err = ValidatePrepareImportInput(input)
	require.EqualError(t, err, `locked_provider_source_invalid: locked_providers key "aws" lacks a provider namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl`)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, lockedProviderSourceInvalidCode, code)

	input.LockedProviders = map[string]string{
		"hashicorp/aws":                       "6.18.0",
		"registry.terraform.io/hashicorp/aws": "6.18.0",
	}
	canonical, err = canonicalizeLockedProviders(input.LockedProviders)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}, canonical)

	input.LockedProviders["hashicorp/aws"] = "6.19.0"
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, `resolve to "registry.terraform.io/hashicorp/aws"`)
	require.ErrorContains(t, err, "different exact versions")

	input.LockedProviders = map[string]string{"unsafe\nsource": "6.18.0"}
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.NotContains(t, err.Error(), "unsafe")
	require.NotContains(t, err.Error(), "\n")

	input.LockedProviders = map[string]string{"registry.terraform.io/hashicorp/aws": ">= 6.0"}
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, "exact selected version")
}

func TestPrepareImportInvalidSourceFailsBeforeClientUse(t *testing.T) {
	input := PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders: map[string]string{"aws": "6.18.0"},
	}
	result, err := ReadPrepareImport(context.Background(), nil, input)
	require.Nil(t, result)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.ErrorContains(t, err, "lacks a provider namespace")
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

func TestImportInputsAndResultsHaveNoApplicationSizeLimit(t *testing.T) {
	selections := make([]ImportSelection, 101)
	for i := range selections {
		selections[i] = ImportSelection{QueryResultMarker: fmt.Sprintf("qrm1-%064x", i), ManagedType: "aws_s3_bucket"}
	}
	input := PrepareImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		QueryRunID:       "qry-example",
		Selections:       selections,
		LockedProviders:  map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"},
	}
	require.True(t, validNormalizedPrepareImportInput(input))

	text, err := FormatImportResult(PrepareImportResult{Status: "prepared", NextAction: strings.Repeat("x", 256<<10)})
	require.NoError(t, err)
	require.Contains(t, text, `"status":"prepared"`)
}

func TestImportInputDoesNotAssumeHCPIdentifierFormats(t *testing.T) {
	input := PrepareImportInput{
		OrganizationName: "organization.example",
		WorkspaceName:    "workspace/name",
		WorkspaceID:      "opaque workspace id",
		QueryRunID:       "opaque query run id",
		Selections:       []ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders:  map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"},
	}
	require.True(t, validNormalizedPrepareImportInput(input))
}

func TestImportArgumentDecodingHasNoApplicationSizeOrDepthLimit(t *testing.T) {
	type arguments struct {
		Value any `json:"value"`
	}
	deep := any("value")
	for range 65 {
		deep = []any{deep}
	}
	raw, err := json.Marshal(map[string]any{"value": map[string]any{"large": strings.Repeat("x", 512<<10), "deep": deep}})
	require.NoError(t, err)

	var decoded arguments
	require.NoError(t, decodeImportArguments(json.RawMessage(raw), &decoded))
	require.NotNil(t, decoded.Value)
}

func TestImportArgumentDecodingRetainsStrictStructureChecks(t *testing.T) {
	type arguments struct {
		Value string `json:"value"`
	}
	var decoded arguments
	require.EqualError(t, decodeImportArguments(json.RawMessage(`{"value":"one","value":"two"}`), &decoded), "query_evidence_invalid")
	require.EqualError(t, decodeImportArguments(json.RawMessage(`{"value":"one","unknown":true}`), &decoded), "import_arguments_invalid")
}

func TestVerifyImportRejectsDuplicateDetailAddresses(t *testing.T) {
	_, err := ReadVerifyImportPlan(context.Background(), nil, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", DetailAddresses: []string{"aws_s3_bucket.sample", "aws_s3_bucket.sample"}})
	require.EqualError(t, err, "verify_input_invalid")
}

func TestImportToolOutputsAddContextAndPreserveDiagnosticCodes(t *testing.T) {
	_, _, err := PrepareImportToolOutput(context.Background(), nil, PrepareImportInput{})
	require.EqualError(t, err, "preparing the import selection: prepare_input_invalid")
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, prepareInputInvalidCode, code)

	_, _, err = VerifyImportPlanToolOutput(context.Background(), nil, VerifyImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		RunID:            "run-example",
		DetailAddresses:  []string{"aws_s3_bucket.sample", "aws_s3_bucket.sample"},
	})
	require.EqualError(t, err, "verifying the import plan: verify_input_invalid")
	code, ok = searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, verifyInputInvalidCode, code)
}

func TestTerraformClientUnavailableErrorPreservesCodeAndCause(t *testing.T) {
	cause := errors.New("session is not configured")
	err := TerraformClientUnavailableError(cause)
	require.EqualError(t, err, "getting Terraform client: client_unavailable")
	require.ErrorIs(t, err, cause)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, clientUnavailableCode, code)
}

func TestVerifyImportRequestDoesNotAssumeHCPIdentifierFormats(t *testing.T) {
	details, err := validateNormalizedVerifyImportRequest(VerifyImportInput{
		OrganizationName: "organization.example",
		WorkspaceName:    "workspace/name",
		RunID:            "opaque run id",
	}, defaultVerifyImportPollConfig())
	require.NoError(t, err)
	require.Empty(t, details)
}

func TestVerifyImportDetailAddressesHaveNoApplicationCountLimit(t *testing.T) {
	details := make([]string, 21)
	for i := range details {
		details[i] = fmt.Sprintf("aws_s3_bucket.sample%d", i)
	}
	var runReads int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-example","attributes":{"name":"target"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/runs/run-example":
			runReads++
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)

	_, err = ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", DetailAddresses: details})
	require.EqualError(t, err, "run_read_unavailable")
	require.Equal(t, 1, runReads)
}

func TestVerifyImportRunGuardsPlanJSONRead(t *testing.T) {
	for _, tc := range []struct {
		name, workspace, planOnly, autoApply, want, required, forbidden string
	}{
		{"wrong workspace", "ws-other", "true", "false", "run_workspace_mismatch", "exact Run", "latest"},
		{"missing plan-only", "ws-example", "null", "false", "run_not_plan_only", "plan-only=true", "classify this Run as successful"},
		{"missing auto-apply", "ws-example", "true", "null", "run_not_plan_only", "auto-apply=false", "classify this Run as successful"},
		{"auto-apply enabled", "ws-example", "true", "true", "run_not_plan_only", "auto-apply=false", "classify this Run as successful"},
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
			c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
			require.NoError(t, err)
			out, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"})
			require.NoError(t, err)
			require.Equal(t, "blocked", out.Status)
			require.Equal(t, []string{tc.want}, out.Diagnostics)
			require.Contains(t, out.NextAction, tc.required)
			require.NotContains(t, out.NextAction, tc.forbidden)
			require.Nil(t, out.Plan)
			require.Nil(t, out.Changes)
			require.Nil(t, out.ObjectIdentity)
			require.Zero(t, planReads, "no Plan metadata or JSON read is allowed")
		})
	}
}

func TestVerifyImportReturnsPendingWithinWaitBudget(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-example","attributes":{"name":"target"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/runs/run-example":
			_, _ = fmt.Fprint(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":"planning","plan-only":true,"auto-apply":false},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-example"}},"plan":{"data":null},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-example"}}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)

	started := time.Now()
	out, err := readVerifyImportPlanWithConfig(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"}, verifyImportPollConfig{wait: 100 * time.Millisecond, interval: 10 * time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, "pending", out.Status)
	require.Equal(t, []string{"run_pending"}, out.Diagnostics)
	require.Contains(t, out.NextAction, "same Run ID")
	require.Contains(t, out.NextAction, "retry_after_seconds")
	require.Contains(t, out.NextAction, "Do not create another Run")
	require.Less(t, time.Since(started), time.Second)
}

func TestVerifyImportWaitLeavesHTTPWriteHeadroom(t *testing.T) {
	require.Less(t, verifyImportWait, 30*time.Second)
}

func TestPrepareImportBlockedActionsAreStateSpecific(t *testing.T) {
	tests := []struct {
		code, required, forbidden string
	}{
		{"target_mismatch", "intended target workspace", "change workspace mode"},
		{"target_execution_mode_not_hcp", "supported HCP execution mode", "change workspace mode automatically"},
		{"target_terraform_below_floor", "Terraform-version compatibility", "change the workspace version"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			action := prepareImportBlockedAction(test.code)
			require.Contains(t, action, test.required)
			if test.code == "target_mismatch" {
				require.NotContains(t, action, test.forbidden)
			} else {
				require.Contains(t, action, "do not")
				require.Contains(t, action, test.forbidden)
			}
			require.NotContains(t, action, "terraform apply")
		})
	}
}

func TestSafePlanPreflightActionsUseExactPlanWithoutClaimingFindings(t *testing.T) {
	for _, code := range []string{
		"plan_json_empty", "plan_json_incomplete", "plan_json_errored",
		"plan_format_unsupported", "plan_terraform_version_unavailable", "plan_change_limit",
	} {
		t.Run(code, func(t *testing.T) {
			action := safePlanPreflightAction(code)
			require.Contains(t, action, "same exact Plan")
			require.Contains(t, action, "run_ui_url")
			require.Contains(t, action, "user's choice")
			require.Contains(t, action, "available get_plan_json_output")
			require.Contains(t, action, "this result's plan_id")
			require.Contains(t, action, "raw Plan JSON may contain sensitive values")
			require.NotContains(t, action, "Plan actions are")
			require.NotContains(t, action, "object identity")
			require.NotContains(t, action, "terraform apply")
		})
	}
	unsupported := safePlanPreflightAction("plan_terraform_version_unsupported")
	require.Contains(t, unsupported, "below-1.5")
	require.Contains(t, unsupported, "separate approval")
	require.Contains(t, unsupported, "do not change settings automatically")
}

func TestVerifyImportPreservesCallerCancellation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-example","attributes":{"name":"target"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/runs/run-example":
			_, _ = fmt.Fprint(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":"planning","plan-only":true,"auto-apply":false},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-example"}},"plan":{"data":null},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-example"}}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	out, err := readVerifyImportPlanWithConfig(ctx, c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"}, verifyImportPollConfig{wait: time.Second, interval: 500 * time.Millisecond})
	require.Nil(t, out)
	require.EqualError(t, err, "verify_canceled")
}
