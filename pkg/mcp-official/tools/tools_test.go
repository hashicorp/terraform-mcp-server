// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func newOfficialSearchSession(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "search-test", Version: "0"}, nil)
	RegisterTools(srv, slog.Default(), toolsets.NewToolsetFilter([]string{toolsets.Search}))
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "search-test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return ctx, session
}

func TestOfficialSearchRegistrationAndConfigurationCall(t *testing.T) {
	ctx, session := newOfficialSearchSession(t)

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	want := map[string]bool{
		"generate_query_configuration": false, "provider_list_schema_list": false,
		"execute_query": false, "get_query_status": false, "get_query_summary": false,
		"prepare_import": false, "verify_import_plan": false,
	}
	require.Len(t, listed.Tools, len(want), "search-only filter must not expose other tools")
	for _, tool := range listed.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
			require.NotNil(t, tool.InputSchema)
			if tool.Name == "prepare_import" || tool.Name == "verify_import_plan" {
				require.NotNil(t, tool.OutputSchema)
				require.True(t, tool.Annotations.ReadOnlyHint)
				require.Contains(t, tool.Description, "Run")
			}
		}
	}
	for name, found := range want {
		require.True(t, found, "official server did not register %s", name)
	}

	args := map[string]string{
		"list_resource_schemas": `{"example_list":{"block":{"attributes":{}}}}`,
		"provider_name":         "example", "provider_namespace": "hashicorp", "provider_version": "1.0.0",
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "generate_query_configuration", Arguments: args})
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	require.False(t, result.IsError, "tool result: %s", result.Content[0].(*mcp.TextContent).Text)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "example_list")

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v2/queries/qry-test" && r.URL.Path != "/api/v2/queries/qry-errored" {
			http.Error(w, "unexpected backend path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.URL.Path == "/api/v2/queries/qry-errored" {
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-errored","attributes":{"status":"errored","terraform-version":"1.14.0","generate-config-out":false}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","terraform-version":"1.14.0","generate-config-out":false}}}`)
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	statusResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_status", Arguments: map[string]string{"query_run_id": "qry-test"}})
	require.NoError(t, err)
	require.False(t, statusResult.IsError, "status result: %#v", statusResult.Content)
	require.Equal(t, "qry-test", statusResult.StructuredContent.(map[string]any)["query_run_id"])
	require.Equal(t, true, statusResult.StructuredContent.(map[string]any)["terminal"])
	require.Contains(t, statusResult.Content[0].(*mcp.TextContent).Text, "get_query_summary")
	failedRun, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_status", Arguments: map[string]string{"query_run_id": "qry-errored"}})
	require.NoError(t, err)
	require.False(t, failedRun.IsError, "observed Terraform failures are factual status results")
	require.Equal(t, "errored", failedRun.StructuredContent.(map[string]any)["status"])
	missingRun, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_status", Arguments: map[string]string{"query_run_id": "qry-missing"}})
	require.NoError(t, err)
	require.True(t, missingRun.IsError, "unreadable backend QueryRun must not be pending")
}

func TestOfficialSearchScopedCatalogAndQuerySubmission(t *testing.T) {
	var posted atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/ws":
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`)
		case "/api/v2/search/provider-versions":
			if r.URL.Query().Get("filter[organization][name]") != "org" {
				http.Error(w, "catalog request missing organization scope", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":[{"type":"provider-versions","id":"hashicorp/example/1.0.0","attributes":{"namespace":"hashicorp","name":"example","version":"1.0.0"}}]}`)
		case "/api/v2/search/provider-versions/hashicorp/example/1.0.0":
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":{"type":"search-compatible-provider-versions","id":"hashicorp/example/1.0.0","attributes":{"list-resource-schemas":{"example_list":{"block":{"attributes":{}}}}}}}`)
		case "/api/v2/search/no-code-query":
			if r.Method != http.MethodPost {
				http.Error(w, "expected POST", http.StatusMethodNotAllowed)
				return
			}
			posted.Store(true)
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"data":{"type":"no-code-queries","id":"ncqry-test","artifact_url":"https://archivist.example/signed?secret=sentinel","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-test"}}}}}`)
		default:
			http.Error(w, "unexpected backend path", http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	ctx, session := newOfficialSearchSession(t)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "ws"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "catalog result: %#v", result.Content)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "supported_providers")
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "ws", "provider_namespace": "hashicorp", "provider_name": "example"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "schema detail without provider metadata: %#v", result.Content)
	var fetched struct {
		Namespace string          `json:"namespace"`
		Name      string          `json:"name"`
		Version   string          `json:"version"`
		Schemas   json.RawMessage `json:"list_resource_schemas"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &fetched))
	require.Equal(t, "hashicorp", fetched.Namespace)
	require.Equal(t, "example", fetched.Name)
	require.Equal(t, "1.0.0", fetched.Version)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "generate_query_configuration", Arguments: map[string]any{"provider_namespace": fetched.Namespace, "provider_name": fetched.Name, "provider_version": fetched.Version, "list_resource_schemas": string(fetched.Schemas), "resource_types": "example_list"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "FETCH metadata must generate a guide: %#v", result.Content)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "example_list")
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "not-found"}})
	require.NoError(t, err)
	require.True(t, result.IsError, "missing workspace must not be treated as authorized")

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "execute_query", Arguments: map[string]any{
		"terraform_org_name": "org", "workspace_name": "ws",
		"query_configuration": `{"no_code_query_providers":[{"namespace":"hashicorp","name":"example","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"example_list"}}]}]}`,
	}})
	require.NoError(t, err)
	require.False(t, result.IsError, "execute result: %#v", result.Content)
	require.True(t, posted.Load())
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "qry-test")
	require.Equal(t, "qry-test", result.StructuredContent.(map[string]any)["query_run_id"])
	require.JSONEq(t, `{"query_run_id":"qry-test"}`, result.Content[0].(*mcp.TextContent).Text)
	require.NotContains(t, fmt.Sprint(result.StructuredContent), "sentinel")
}

func TestOfficialImportVerifyObservedFailureAndNoPlanJSON(t *testing.T) {
	var planReads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-target","attributes":{"name":"target"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/runs/run-example":
			_, _ = fmt.Fprint(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":"errored","plan-only":true,"auto-apply":false},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-target"}}}}}`)
		default:
			planReads.Add(1)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	ctx, session := newOfficialSearchSession(t)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "verify_import_plan", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "target", "run_id": "run-example"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "observed failed Run is a factual domain result")
	data := result.StructuredContent.(map[string]any)
	require.Equal(t, "failed", data["status"])
	require.Equal(t, "run-example", data["run_id"])
	require.Equal(t, float64(0), float64(planReads.Load()))
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `"run_failed"`)
	// Unknown fields and missing required inputs are tool errors, not domain failures.
	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "verify_import_plan", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "target", "run_id": "run-example", "configuration_version_id": "cv-forbidden"}})
	require.NoError(t, err)
	require.True(t, invalid.IsError)
}

func TestOfficialImportPrepareAndFinishedPlan(t *testing.T) {
	var backend *httptest.Server
	var invalidPlanJSON atomic.Bool
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-source"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs")
		case "/api/v2/workspaces/ws-source":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-source","attributes":{"name":"source"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-source"}}}}}`)
		case "/logs":
			body := []byte("\x02Starting query\n" + `{"type":"provider_installation_start","provider":"aws"}` + "\n" + `{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","identity":{"bucket":"sample"},"identity_version":1}}` + "\n" + `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":1}}` + "\n\x03")
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(body) {
				_, _ = w.Write(body[offset:min(len(body), offset+limit)])
			}
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-target","attributes":{"name":"target","execution-mode":"remote","terraform-version":"1.14.0"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/workspaces/ws-target/current-state-version":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/runs/run-example":
			_, _ = fmt.Fprint(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":"planned_and_finished","plan-only":true,"auto-apply":false},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-target"}},"plan":{"data":{"type":"plans","id":"plan-test"}},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-test"}}}}}`)
		case "/api/v2/plans/plan-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"plans","id":"plan-test","attributes":{"status":"finished"}}}`)
		case "/api/v2/plans/plan-test/json-output":
			if invalidPlanJSON.Load() {
				_, _ = fmt.Fprint(w, `{"private":"sentinel"} invalid-private-sentinel`)
				return
			}
			_, _ = fmt.Fprint(w, `{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[{"address":"aws_s3_bucket.sample","identity_schema_version":1}]}},"resource_changes":[{"address":"aws_s3_bucket.sample","mode":"managed","type":"aws_s3_bucket","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"secret-id-sentinel"},"after_identity":{"bucket":"sample"}}}],"resource_drift":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	ctx, session := newOfficialSearchSession(t)
	summary, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_summary", Arguments: map[string]any{"query_run_id": "  qry-test\t"}})
	require.NoError(t, err)
	require.False(t, summary.IsError)
	require.Equal(t, "qry-test", summary.StructuredContent.(map[string]any)["query_run_id"])
	lists := summary.StructuredContent.(map[string]any)["lists"].([]any)
	marker := lists[0].(map[string]any)["results"].([]any)[0].(map[string]any)["query_result_marker"].(string)
	prep, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "prepare_import", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "target", "query_run_id": "qry-test", "selections": []any{map[string]any{"query_result_marker": marker, "managed_type": "aws_s3_bucket"}}, "locked_providers": map[string]string{"hashicorp/aws": "6.66.0"}}})
	require.NoError(t, err)
	require.False(t, prep.IsError, "prepare: %#v", prep.Content)
	prepared := prep.StructuredContent.(map[string]any)
	require.Equal(t, "prepared", prepared["status"])
	require.Equal(t, "source", prepared["source_workspace_name"])
	require.Equal(t, "ws-target", prepared["workspace_id"])
	require.Equal(t, "6.66.0", prepared["import_context"].(map[string]any)["locked_providers"].(map[string]any)["registry.terraform.io/hashicorp/aws"])
	verify, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "verify_import_plan", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "target", "run_id": "run-example", "import_context": prepared["import_context"], "bindings": []any{map[string]any{"query_result_marker": marker, "target_address": "aws_s3_bucket.sample"}}}})
	require.NoError(t, err)
	require.False(t, verify.IsError, "verify: %#v", verify.Content)
	facts := verify.StructuredContent.(map[string]any)
	require.Equal(t, "plan_available", facts["status"])
	require.Equal(t, float64(1), facts["object_identity"].(map[string]any)["matched"])
	require.NotContains(t, fmt.Sprint(verify.Content), "secret-id-sentinel")
	require.NotContains(t, fmt.Sprint(verify.StructuredContent), "secret-id-sentinel")

	invalidPlanJSON.Store(true)
	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "verify_import_plan", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "target", "run_id": "run-example", "import_context": prepared["import_context"], "bindings": []any{map[string]any{"query_result_marker": marker, "target_address": "aws_s3_bucket.sample"}}}})
	require.NoError(t, err)
	require.True(t, invalid.IsError)
	require.Nil(t, invalid.StructuredContent)
	require.Contains(t, invalid.Content[0].(*mcp.TextContent).Text, "plan_json_invalid")
	require.NotContains(t, fmt.Sprint(invalid.Content), "invalid-private-sentinel")
	require.NotContains(t, fmt.Sprint(invalid.Content), `"status":"blocked"`)
}

func TestOfficialSearchQuerySummarySelectableAndNonselectable(t *testing.T) {
	for _, tc := range []struct {
		name, version, status string
		selectable            bool
	}{
		{"valid version zero", "0", "finished", true},
		{"missing identity version", "null", "finished", false},
		{"terminal failed query", "0", "errored", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var backend *httptest.Server
			backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/queries/qry-test":
					_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":%q,"log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, tc.status, backend.URL+"/logs")
				case "/api/v2/workspaces/ws-test":
					_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
				case "/api/v2/search/no-code-query/ncqry-test":
					_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
				case "/logs":
					body := []byte("\x02Starting query\n" + `{"type":"provider_installation_start","provider":"aws"}` + "\n" + `{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"first","identity":{"bucket":"first"},"identity_version":` + tc.version + `,"resource_object":{"tags":{"team":"search"}},"config":"resource \"aws_s3_bucket\" \"first\" {}","import_config":"import { id = \"must-not-be-returned\" }"}}` + "\n" + `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":1}}` + "\n\x03")
					offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
					limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
					if offset < len(body) {
						_, _ = w.Write(body[offset:min(len(body), offset+limit)])
					}
				default:
					http.Error(w, "not found", http.StatusNotFound)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
			ctx, session := newOfficialSearchSession(t)
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_summary", Arguments: map[string]any{"query_run_id": "qry-test"}})
			require.NoError(t, err)
			require.False(t, result.IsError, "result: %#v", result.Content)
			structured := result.StructuredContent.(map[string]any)
			require.Equal(t, tc.selectable, structured["selectable"])
			text := result.Content[0].(*mcp.TextContent).Text
			if tc.selectable {
				require.Contains(t, text, `"query_result_marker":"qrm1-`)
				require.Equal(t, "org", structured["source_organization_name"])
				rows := structured["lists"].([]any)[0].(map[string]any)["results"].([]any)
				observed := rows[0].(map[string]any)["observed_attributes"].(map[string]any)
				require.Equal(t, "search", observed["tags"].(map[string]any)["team"])
				require.Equal(t, true, rows[0].(map[string]any)["attributes_captured"])
				require.Equal(t, `resource "aws_s3_bucket" "first" {}`, rows[0].(map[string]any)["configuration"])
				require.Contains(t, text, `"observed_attributes":{"tags":{"team":"search"}}`)
				require.Contains(t, text, `"configuration":"resource \"aws_s3_bucket\" \"first\" {}"`)
				require.NotContains(t, text, "must-not-be-returned")
			} else {
				require.Equal(t, "nonselectable", structured["status"])
				require.NotContains(t, text, "query_result_marker")
				require.NotContains(t, text, "first")
			}
			require.False(t, strings.Contains(text, "/logs"))
		})
	}
}
