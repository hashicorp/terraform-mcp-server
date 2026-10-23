// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	legacymcp "github.com/mark3labs/mcp-go/mcp"
	legacyserver "github.com/mark3labs/mcp-go/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const prepareResourceDraft = "\nresource \"aws_s3_bucket\" \"first\" { bucket = \"selected-draft-secret\" }\n"
const prepareImportDraft = "\nimport { to = aws_s3_bucket.first id = \"review-before-use\" }\n"

func prepareSelectedMarker() string {
	// The source marker's canonical payload; no new QueryRun or summary read is
	// needed to exercise preflight rejection before evidence access.
	return fmt.Sprintf("qrm1-%x", sha256.Sum256([]byte(`{"query_run_id":"qry-test","provider_source":"registry.terraform.io/hashicorp/aws","provider_version":"6.66.0","list_type":"aws_s3_buckets","identity":{"bucket":"first"}}`)))
}

func prepareLogFixture(t *testing.T, drafts bool) string {
	t.Helper()
	var records []string
	for _, bucket := range []string{"first", "unselected"} {
		found := map[string]any{"address": "list.aws_s3_buckets.all", "resource_type": "aws_s3_buckets", "identity": map[string]any{"bucket": bucket}, "identity_version": 0, "resource_object": map[string]any{"secret": "resource-object-secret-sentinel"}}
		if drafts {
			found["config"], found["import_config"] = prepareResourceDraft, prepareImportDraft
			if bucket == "unselected" {
				found["config"], found["import_config"] = "unselected-invalid-private-hcl", "unselected-private-import"
			}
		}
		data, err := json.Marshal(map[string]any{"type": "list_resource_found", "list_resource_found": found})
		require.NoError(t, err)
		records = append(records, string(data))
	}
	return strings.Join(append(records, `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":2}}`), "\n") + "\n"
}

func TestPrepareImportTransportParityAndPreflight(t *testing.T) {
	valid := prepareLogFixture(t, true)
	for _, tc := range []struct {
		name, body, mode, version, targetOrg, sourceOrg, workspaceID, failPath, diagnostic string
		blocked, beforeLogs, noDrafts, historical                                          bool
	}{
		{name: "selected only exact drafts and context", body: valid},
		{name: "generation does not guarantee drafts", body: prepareLogFixture(t, false), noDrafts: true},
		{name: "agent execution", body: valid, mode: "agent"},
		{name: "unknown Terraform version is not compatibility proof", body: valid, version: "latest"},
		{name: "historical producing Run only", body: valid, historical: true},
		{name: "target id mismatch", body: valid, workspaceID: "ws-wrong", diagnostic: "target_mismatch", blocked: true, beforeLogs: true},
		{name: "local execution", body: valid, mode: "local", diagnostic: "target_execution_mode_not_hcp", blocked: true, beforeLogs: true},
		{name: "old Terraform", body: valid, version: "1.4.9", diagnostic: "target_terraform_below_floor", blocked: true, beforeLogs: true},
		{name: "target ownership mismatch", body: valid, targetOrg: "other", diagnostic: "target_ownership_unverified", beforeLogs: true},
		{name: "source organization mismatch", body: valid, sourceOrg: "other", diagnostic: "query_organization_mismatch", beforeLogs: true},
		{name: "target unavailable", body: valid, failPath: "/api/v2/organizations/org/workspaces/target", diagnostic: "target_read_unavailable", beforeLogs: true},
		{name: "log unavailable", body: valid, failPath: "/logs", diagnostic: "query_log_unavailable", beforeLogs: true},
		{name: "late malformed evidence", body: valid + `{"type":"progress",`, diagnostic: "query_evidence_invalid"},
		{name: "late diagnostic error", body: valid + `{"type":"diagnostic","diagnostic":{"severity":"error","detail":"backend-secret-sentinel"}}`, diagnostic: "query_reported_error"},
		{name: "late completion failure", body: strings.Replace(valid, `"total":2`, `"total":3`, 1), diagnostic: "query_evidence_incomplete"},
		{name: "malformed unselected identity", body: strings.Replace(valid, `"identity":{"bucket":"unselected"}`, `"identity":[]`, 1), diagnostic: "query_evidence_invalid"},
		{name: "invalid selected HCL", body: strings.Replace(valid, `bucket = \"selected-draft-secret\" }`, `bucket = \"selected-draft-secret\" `, 1), diagnostic: "query_generated_configuration_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mode == "" {
				tc.mode = "remote"
			}
			if tc.version == "" {
				tc.version = "1.14.0"
			}
			if tc.targetOrg == "" {
				tc.targetOrg = "org"
			}
			if tc.sourceOrg == "" {
				tc.sourceOrg = "org"
			}
			source, reads := summaryReadOnlyBackend(t, "finished", tc.sourceOrg, tc.body, tc.failPath)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method, "preparation must never mutate")
				w.Header().Set("Content-Type", "application/vnd.api+json")
				if r.URL.Path == tc.failPath {
					http.Error(w, "backend-secret-sentinel", http.StatusNotFound)
					return
				}
				switch r.URL.Path {
				case "/api/v2/organizations/org/workspaces/target":
					_, _ = fmt.Fprintf(w, `{"data":{"type":"workspaces","id":"ws-target","attributes":{"name":"target","execution-mode":%q,"terraform-version":%q},"relationships":{"organization":{"data":{"type":"organizations","id":%q}}}}}`, tc.mode, tc.version, tc.targetOrg)
				case "/api/v2/workspaces/ws-target/current-state-version":
					if !tc.historical {
						http.NotFound(w, r)
						return
					}
					_, _ = fmt.Fprint(w, `{"data":{"type":"state-versions","id":"sv-producing","relationships":{"run":{"data":{"type":"runs","id":"run-producing"}}}}}`)
				case "/api/v2/runs/run-producing":
					require.True(t, tc.historical)
					_, _ = fmt.Fprint(w, `{"data":{"type":"runs","id":"run-producing","attributes":{"terraform-version":"1.14.0"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-target"}},"plan":{"data":{"type":"plans","id":"plan-producing"}}}}}`)
				case "/api/v2/plans/plan-producing":
					_, _ = fmt.Fprint(w, `{"data":{"type":"plans","id":"plan-producing"}}`)
				case "/api/v2/runs/run-producing/plan/json-schema":
					_, _ = fmt.Fprint(w, `{"format_version":"1.0","provider_schemas":{"registry.terraform.io/hashicorp/aws":{"resource_schemas":{"aws_s3_bucket":{"version":0}},"resource_identity_schemas":{"aws_s3_bucket":{"version":0}}}}}`)
				default:
					source.Config.Handler.ServeHTTP(w, r)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org,other")
			var officialLogs, legacyLogs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&officialLogs, nil))
			ctx, session := newOfficialSearchSession(t, middleware.OrganizationAllowlist([]string{"org", "other"}, logger), middleware.ToolLogging(logger))
			args := map[string]any{"terraform_org_name": "org", "workspace_name": "target", "query_run_id": " qry-test\t", "workspace_id": tc.workspaceID,
				"selections": []any{map[string]any{"query_result_marker": prepareSelectedMarker(), "managed_type": "aws_s3_bucket"}}, "locked_providers": map[string]string{"hashicorp/aws": "6.18.0"}}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "prepare_import", Arguments: args})
			require.NoError(t, err)
			legacyLogger := log.New()
			legacyLogger.SetOutput(&legacyLogs)
			legacyLogger.SetLevel(log.ErrorLevel)
			legacyCtx := legacyserver.NewMCPServer("prepare-test", "0").WithContext(ctx, summaryTestSession{})
			legacyCtx = terraformclient.WithOrganizationAllowlist(legacyCtx, []string{"org", "other"})
			old, err := searchlogic.PrepareImport(legacyLogger).Handler(legacyCtx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Name: "prepare_import", Arguments: args}})
			require.NoError(t, err)
			toolError := tc.diagnostic != "" && !tc.blocked
			require.Equal(t, toolError, result.IsError)
			require.Equal(t, result.IsError, old.IsError)
			text := result.Content[0].(*mcp.TextContent).Text
			require.Equal(t, old.Content[0].(legacymcp.TextContent).Text, text)
			if tc.beforeLogs {
				require.Zero(t, reads.Load(), "preflight must precede log reads")
			}
			for _, value := range []string{"unselected-private", "unselected-invalid", "resource_object", "resource-object-secret-sentinel", "signed-secret-sentinel", "backend-secret-sentinel", backend.URL, source.URL} {
				require.NotContains(t, text, value)
				require.NotContains(t, officialLogs.String(), value)
				require.NotContains(t, legacyLogs.String(), value)
			}
			for _, logs := range []string{officialLogs.String(), legacyLogs.String()} {
				require.NotContains(t, logs, "selected-draft-secret")
				require.NotContains(t, logs, prepareSelectedMarker())
			}
			if toolError {
				require.Nil(t, result.StructuredContent)
				require.Nil(t, old.StructuredContent)
				require.Contains(t, text, tc.diagnostic)
				require.NotContains(t, text, "selected-draft-secret")
				return
			}
			for _, structured := range []any{result.StructuredContent, old.StructuredContent} {
				encoded, err := json.Marshal(structured)
				require.NoError(t, err)
				require.JSONEq(t, text, string(encoded))
			}
			var prepared searchlogic.PrepareImportResult
			require.NoError(t, json.Unmarshal([]byte(text), &prepared))
			if tc.blocked {
				require.Equal(t, "blocked", prepared.Status)
				require.Equal(t, []string{tc.diagnostic}, prepared.Diagnostics)
				require.Empty(t, prepared.SelectedResults)
				require.Nil(t, prepared.ImportContext)
				return
			}
			require.Equal(t, "prepared", prepared.Status)
			require.True(t, prepared.GenerateConfigOut)
			require.Len(t, prepared.SelectedResults, 1)
			selected := prepared.SelectedResults[0]
			require.Equal(t, !tc.noDrafts, selected.ConfigurationGenerated)
			if tc.noDrafts {
				require.Empty(t, selected.Configuration)
				require.Empty(t, selected.ImportConfig)
			} else {
				require.Equal(t, prepareResourceDraft, selected.Configuration)
				require.Equal(t, prepareImportDraft, selected.ImportConfig)
			}
			require.NotContains(t, text, "configuration_attributes")
			carried := prepared.ImportContext
			require.NotNil(t, carried)
			require.Len(t, carried.SelectedResults, 1)
			require.Equal(t, selected.Identity, carried.SelectedResults[0].Identity)
			require.Equal(t, selected.IdentityVersion, carried.SelectedResults[0].SearchIdentityVersion)
			require.Equal(t, selected.Provider, carried.Providers[selected.ResourceType])
			require.Equal(t, prepareSelectedMarker(), carried.SelectedResults[0].QueryResultMarker)
			require.Equal(t, "aws_s3_bucket", carried.SelectedResults[0].ManagedType)
			require.Equal(t, "qry-test", carried.QueryRunID)
			require.Equal(t, "ws-target", carried.Target.WorkspaceID)
			require.Equal(t, map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}, carried.LockedProviders)
			require.Regexp(t, `^sha256:[a-f0-9]{64}$`, carried.SelectionDigest)
			require.Equal(t, "6.66.0", prepared.Types[0].SearchProviderVersion)
			require.Equal(t, "6.18.0", prepared.Types[0].LockedProviderVersion)
			require.Equal(t, "unverified", prepared.Types[0].IdentityCompatibility)
			if tc.historical {
				require.Equal(t, "run-producing", prepared.SchemaSource.RunID)
				require.Equal(t, "supported_historically", prepared.Types[0].IdentitySupport)
			} else {
				require.Nil(t, prepared.SchemaSource)
				require.Equal(t, "unknown", prepared.Types[0].IdentitySupport)
			}
		})
	}
}

func TestPrepareImportStrictWireInputsBeforeBackend(t *testing.T) {
	// Any accidental client/backend access fails rather than finding local credentials.
	t.Setenv("TFE_TOKEN", "")
	t.Setenv("TFE_ADDRESS", "http://127.0.0.1:1")
	t.Setenv("TF_TOKEN_app_terraform_io", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TF_CLI_CONFIG_FILE", t.TempDir()+"/absent.tfrc")
	ctx, session := newOfficialSearchSession(t)
	for _, tc := range []struct{ name, extra, selection, locked string }{
		{"unknown top level", `,"run_id":"not-a-prepare-input"`, `{"query_result_marker":"` + prepareSelectedMarker() + `","managed_type":"aws_s3_bucket"}`, `{"hashicorp/aws":"6.18.0"}`},
		{"unknown selection field", "", `{"query_result_marker":"` + prepareSelectedMarker() + `","managed_type":"aws_s3_bucket","target_address":"not-a-prepare-input"}`, `{"hashicorp/aws":"6.18.0"}`},
		{"invalid marker", "", `{"query_result_marker":"not-a-marker","managed_type":"aws_s3_bucket"}`, `{"hashicorp/aws":"6.18.0"}`},
		{"constraint not exact release", "", `{"query_result_marker":"` + prepareSelectedMarker() + `","managed_type":"aws_s3_bucket"}`, `{"hashicorp/aws":">= 6.0"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := json.RawMessage(fmt.Sprintf(`{"terraform_org_name":"org","workspace_name":"target","query_run_id":"qry-test","selections":[%s],"locked_providers":%s%s}`, tc.selection, tc.locked, tc.extra))
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "prepare_import", Arguments: args})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			logger := log.New()
			var logs bytes.Buffer
			logger.SetOutput(&logs)
			old, err := searchlogic.PrepareImport(logger).Handler(ctx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Arguments: args}})
			require.NoError(t, err)
			require.True(t, old.IsError)
			require.Nil(t, old.StructuredContent)
			for _, text := range []string{result.Content[0].(*mcp.TextContent).Text, old.Content[0].(legacymcp.TextContent).Text} {
				require.NotContains(t, text, "client_unavailable", "input errors must precede acquiring credentials")
				require.NotContains(t, text, "127.0.0.1")
			}
			require.NotContains(t, logs.String(), prepareSelectedMarker())
		})
	}
}
