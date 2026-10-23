// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestSearchToImportVerificationTransportParity(t *testing.T) {
	source, logReads := summaryReadOnlyBackend(t, "finished", "org", summaryLogFixture("1"), "")
	var scenario atomic.Value
	scenario.Store("matched")
	var submissions, planReads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.Method != http.MethodGet {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v2/search/no-code-query" {
				t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			submissions.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"data":{"type":"no-code-queries","id":"ncqry-test","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-test"}}}}}`)
			return
		}
		mode := scenario.Load().(string)
		switch r.URL.Path {
		case "/api/v2/organizations/org/workspaces/ws":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`)
		case "/api/v2/search/provider-versions":
			require.Equal(t, "org", r.URL.Query().Get("filter[organization][name]"))
			_, _ = fmt.Fprint(w, `{"data":[{"type":"provider-versions","id":"hashicorp/aws/6.66.0","attributes":{"namespace":"hashicorp","name":"aws","version":"6.66.0"}}]}`)
		case "/api/v2/search/provider-versions/hashicorp/aws/6.66.0":
			_, _ = fmt.Fprint(w, `{"data":{"type":"search-compatible-provider-versions","id":"hashicorp/aws/6.66.0","attributes":{"list-resource-schemas":{"aws_s3_buckets":{"block":{"attributes":{}}}}}}}`)
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-target","attributes":{"name":"target","execution-mode":"remote","terraform-version":"1.14.0"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/workspaces/ws-target/current-state-version":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/runs/run-exact":
			status, workspace, flags, relations := "planned_and_finished", "ws-target", `"plan-only":true,"auto-apply":false`, `,"plan":{"data":{"type":"plans","id":"plan-exact"}},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-exact"}}`
			switch mode {
			case "failed":
				status = "errored"
			case "pending", "lagging failed Plan":
				status = "planning"
			case "wrong workspace":
				workspace = "ws-other"
			case "missing plan-only":
				flags = `"auto-apply":false`
			case "missing auto-apply":
				flags = `"plan-only":true`
			case "auto apply":
				flags = `"plan-only":true,"auto-apply":true`
			case "missing associations":
				relations = ""
			}
			_, _ = fmt.Fprintf(w, `{"data":{"type":"runs","id":"run-exact","attributes":{"status":%q,%s},"relationships":{"workspace":{"data":{"type":"workspaces","id":%q}}%s}}}`, status, flags, workspace, relations)
		case "/api/v2/plans/plan-exact":
			id, status := "plan-exact", "finished"
			if mode == "wrong Plan" {
				id = "plan-other"
			} else if mode == "pending" {
				status = "running"
			} else if mode == "lagging failed Plan" {
				status = "errored"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"type":"plans","id":%q,"attributes":{"status":%q}}}`, id, status)
		case "/api/v2/plans/plan-exact/json-output":
			planReads.Add(1)
			if mode == "malformed Plan" {
				_, _ = fmt.Fprint(w, `{"private":"invalid-private-sentinel"} trailing-private-sentinel`)
				return
			}
			identity := `,"after_identity":{"bucket":"first"}`
			if mode == "unknown identity" {
				identity = ""
			} else if mode == "mismatched identity" {
				identity = `,"after_identity":{"bucket":"different-private-sentinel"}`
			}
			_, _ = fmt.Fprintf(w, `{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[{"address":"aws_s3_bucket.first","identity_schema_version":1}]}},"resource_changes":[{"address":"aws_s3_bucket.first","mode":"managed","type":"aws_s3_bucket","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"import-id-private-sentinel"},"after":{"secret":"plan-value-private-sentinel"}%s}}],"resource_drift":[]}`, identity)
		default:
			source.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	var officialLogs, legacyLogs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&officialLogs, nil))
	ctx, session := newOfficialSearchSession(t, middleware.OrganizationAllowlist([]string{"org"}, logger), middleware.ToolLogging(logger))
	legacyLogger := log.New()
	legacyLogger.SetOutput(&legacyLogs)
	legacyCtx := legacyserver.NewMCPServer("verify-flow", "0").WithContext(ctx, summaryTestSession{})
	legacyCtx = terraformclient.WithOrganizationAllowlist(legacyCtx, []string{"org"})
	legacyTools := map[string]legacyserver.ServerTool{}
	for _, tool := range []legacyserver.ServerTool{
		searchlogic.ProviderListSchemaList(legacyLogger), searchlogic.GenerateQueryConfiguration(legacyLogger),
		searchlogic.ExecuteQuery(legacyLogger), searchlogic.GetQueryStatus(legacyLogger), searchlogic.GetQuerySummary(legacyLogger),
		searchlogic.PrepareImport(legacyLogger), searchlogic.VerifyImportPlan(legacyLogger),
	} {
		legacyTools[tool.Tool.Name] = tool
	}
	callPair := func(name string, args map[string]any, wantError bool) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		old, err := terraformclient.ToolLoggingMiddleware(legacyLogger)(legacyTools[name].Handler)(legacyCtx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Name: name, Arguments: args}})
		require.NoError(t, err)
		require.Equal(t, wantError, result.IsError, "%s: %v", name, result.Content)
		require.Equal(t, wantError, old.IsError, "%s: %v", name, old.Content)
		text := result.Content[0].(*mcp.TextContent).Text
		require.Equal(t, old.Content[0].(legacymcp.TextContent).Text, text)
		// Status preserves its source prose followed by the complete JSON fallback.
		if name == "get_query_status" {
			_, text, _ = strings.Cut(text, "\n\n")
		}
		for _, secret := range []string{"invalid-private-sentinel", "trailing-private-sentinel", "import-id-private-sentinel", "plan-value-private-sentinel", "different-private-sentinel"} {
			require.NotContains(t, text, secret)
			require.NotContains(t, officialLogs.String(), secret)
			require.NotContains(t, legacyLogs.String(), secret)
		}
		if wantError {
			require.Nil(t, result.StructuredContent)
			require.Nil(t, old.StructuredContent)
			return nil
		}
		if result.StructuredContent == nil {
			require.Nil(t, old.StructuredContent)
			if name == "generate_query_configuration" {
				return nil
			}
		} else {
			for _, structured := range []any{result.StructuredContent, old.StructuredContent} {
				encoded, err := json.Marshal(structured)
				require.NoError(t, err)
				require.JSONEq(t, text, string(encoded))
			}
		}
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(text), &data))
		return data
	}

	provider := callPair("provider_list_schema_list", map[string]any{"terraform_org_name": "org", "workspace_name": "ws", "provider_namespace": "hashicorp", "provider_name": "aws"}, false)
	schemas, err := json.Marshal(provider["list_resource_schemas"])
	require.NoError(t, err)
	callPair("generate_query_configuration", map[string]any{"list_resource_schemas": string(schemas), "provider_namespace": provider["namespace"], "provider_name": provider["name"], "provider_version": provider["version"], "resource_types": "aws_s3_buckets"}, false)
	configuration := `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no_code_query_resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]}`
	execution := callPair("execute_query", map[string]any{"terraform_org_name": "org", "workspace_name": "ws", "query_configuration": configuration}, false)
	queryArgs := map[string]any{"query_run_id": execution["query_run_id"]}
	require.Equal(t, true, callPair("get_query_status", queryArgs, false)["terminal"])
	summary := callPair("get_query_summary", queryArgs, false)
	marker := summary["lists"].([]any)[0].(map[string]any)["results"].([]any)[0].(map[string]any)["query_result_marker"].(string)
	prepared := callPair("prepare_import", map[string]any{"terraform_org_name": "org", "workspace_name": "target", "query_run_id": execution["query_run_id"], "selections": []any{map[string]any{"query_result_marker": marker, "managed_type": "aws_s3_bucket"}}, "locked_providers": map[string]string{"hashicorp/aws": "6.66.0"}}, false)
	require.Equal(t, "prepared", prepared["status"])
	contextJSON, err := json.Marshal(prepared["import_context"])
	require.NoError(t, err)
	beforeLogs := logReads.Load()
	beforeSubmissions := submissions.Load()
	require.EqualValues(t, 2, beforeSubmissions, "one fixture QueryRun submission per SDK")
	for _, tc := range []struct {
		name, status, diagnostic, identity string
		readPlanJSON, toolError            bool
	}{
		{name: "matched", status: "plan_available", identity: "matched", readPlanJSON: true},
		{name: "unknown identity", status: "plan_available", identity: "unverified", readPlanJSON: true},
		{name: "mismatched identity", status: "plan_available", identity: "mismatched", readPlanJSON: true},
		{name: "malformed Plan", readPlanJSON: true, toolError: true},
		{name: "wrong workspace", status: "blocked", diagnostic: "run_workspace_mismatch"},
		{name: "missing plan-only", status: "blocked", diagnostic: "run_not_plan_only"},
		{name: "missing auto-apply", status: "blocked", diagnostic: "run_not_plan_only"},
		{name: "auto apply", status: "blocked", diagnostic: "run_not_plan_only"},
		{name: "missing associations", status: "blocked", diagnostic: "run_plan_unassociated"},
		{name: "wrong Plan", status: "blocked", diagnostic: "plan_association_unverified"},
		{name: "missing context", status: "import_context_required", diagnostic: "import_context_required"},
		{name: "altered context", status: "blocked", diagnostic: "import_context_digest_mismatch"},
		{name: "bad binding", status: "blocked", diagnostic: "binding_invalid"},
		{name: "failed", status: "failed", diagnostic: "run_failed"},
		{name: "lagging failed Plan", status: "failed", diagnostic: "plan_failed"},
		{name: "pending", status: "pending", diagnostic: "run_pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scenario.Store(tc.name)
			var carried map[string]any
			require.NoError(t, json.Unmarshal(contextJSON, &carried))
			binding := map[string]any{"query_result_marker": marker, "target_address": "aws_s3_bucket.first"}
			args := map[string]any{"terraform_org_name": "org", "workspace_name": "target", "run_id": " run-exact\t", "import_context": carried, "bindings": []any{binding}}
			switch tc.name {
			case "pending", "failed", "missing context":
				delete(args, "import_context")
				delete(args, "bindings")
			case "altered context":
				carried["selection_digest"] = "sha256:" + strings.Repeat("0", 64)
			case "bad binding":
				binding["query_result_marker"] = "qrm1-" + strings.Repeat("0", 64)
			}
			beforePlans := planReads.Load()
			data := callPair("verify_import_plan", args, tc.toolError)
			if tc.readPlanJSON {
				require.Equal(t, beforePlans+2, planReads.Load())
			} else {
				require.Equal(t, beforePlans, planReads.Load(), "guard must precede Plan JSON access")
			}
			require.Equal(t, beforeLogs, logReads.Load(), "verification must never reread QueryRun evidence")
			require.Equal(t, beforeSubmissions, submissions.Load(), "verification must never resubmit")
			if tc.toolError {
				return
			}
			require.Equal(t, tc.status, data["status"])
			if tc.diagnostic != "" {
				require.Contains(t, data["diagnostics"], tc.diagnostic)
			}
			if tc.identity != "" {
				require.Equal(t, float64(1), data["object_identity"].(map[string]any)[tc.identity])
				require.Equal(t, float64(1), data["changes"].(map[string]any)["none"], "no-op actions never imply matched identity")
				require.Equal(t, "run-exact", data["run_id"])
				require.Equal(t, "plan-exact", data["plan_id"])
				require.Equal(t, "cv-exact", data["configuration_version_id"])
			}
			if tc.status == "pending" {
				require.Equal(t, float64(5), data["retry_after_seconds"])
			}
		})
	}
	for _, logs := range []string{officialLogs.String(), legacyLogs.String()} {
		require.NotContains(t, logs, marker)
		require.NotContains(t, logs, string(contextJSON))
		require.NotContains(t, logs, configuration)
	}
}
