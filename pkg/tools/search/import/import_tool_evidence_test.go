// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareImportToolFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*importBackendTest, *importPrepareInput)
	}{
		{"wrong candidate", "selected_candidate_not_found", func(f *importBackendTest, i *importPrepareInput) {
			i.Selections[0].CandidateID = "candidate-" + strings.Repeat("a", 64)
		}},
		{"schema run belongs to another workspace", "schema_source_workspace_mismatch", func(f *importBackendTest, i *importPrepareInput) {
			f.responses["/api/v2/runs/run-schema"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/runs/run-schema"]), "ws-fixture", "ws-other"))
		}},
		{"missing state-producing run", "state_producing_run_missing", func(f *importBackendTest, i *importPrepareInput) {
			f.responses["/api/v2/workspaces/ws-fixture/current-state-version"] = json.RawMessage(`{"data":{"id":"sv-manual","type":"state-versions"}}`)
		}},
		{"state missing or inaccessible", "current_state_unavailable_or_inaccessible", func(f *importBackendTest, i *importPrepareInput) { f.stateStatus = 404 }},
		{"state access denied", "evidence_access_denied", func(f *importBackendTest, i *importPrepareInput) { f.stateStatus = 403 }},
		{"schema missing type", "managed_schema_type_missing", func(f *importBackendTest, i *importPrepareInput) { i.Selections[0].ManagedType = "aws_absent" }},
		{"schema denied", "schema_source_access_denied", func(f *importBackendTest, i *importPrepareInput) { f.schemaStatus = 403 }},
		{"schema pending", "schema_source_pending", func(f *importBackendTest, i *importPrepareInput) { f.schemaStatus = 204 }},
		{"schema missing", "schema_source_unavailable", func(f *importBackendTest, i *importPrepareInput) { f.schemaStatus = 404 }},
		{"CLI unsupported", "schema_source_cli_unsupported", func(f *importBackendTest, i *importPrepareInput) { f.schemaStatus = 422 }},
		{"changed baseline", "baseline_changed", func(f *importBackendTest, i *importPrepareInput) { f.baselineChanged = true }},
		{"changed state", "baseline_changed", func(f *importBackendTest, i *importPrepareInput) { f.stateChanged = true }},
		{"schema provider absent", "schema_provider_missing", func(f *importBackendTest, i *importPrepareInput) {
			f.responses["/schema-download"] = json.RawMessage(`{"format_version":"1.0","provider_schemas":{}}`)
		}},
		{"schema malformed", "managed_schema_invalid", func(f *importBackendTest, i *importPrepareInput) {
			f.responses["/schema-download"] = json.RawMessage(`{"format_version":"1.0","provider_schemas":{"registry.terraform.io/hashicorp/aws":{"resource_schemas":{"aws_iam_role":null}}}}`)
		}},
		{"schema format unsupported", "schema_format_unsupported", func(f *importBackendTest, i *importPrepareInput) {
			f.responses["/schema-download"] = json.RawMessage(`{"format_version":"2.0","provider_schemas":{}}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := importBackendFixture(t)
			input := importFixtureInput(t)
			tc.change(f, &input)
			out := prepareImportTool(context.Background(), f.client, input)
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, tc.code)
			assert.Nil(t, out.Carry, "a blocked preparation returns no carry block")
			encoded, _ := json.Marshal(out)
			assert.NotContains(t, string(encoded), "secret must not leak")
		})
	}
}

func TestPrepareImportToolSchemaIsComplete(t *testing.T) {
	f := importBackendFixture(t)
	selected := `{"version":99,"block":{"attributes":{"name":{"type":"string","required":true}},"block_types":{"settings":{"nesting_mode":"list","min_items":1,"max_items":2,"block":{"attributes":{"values":{"type":["map","string"],"optional":true,"computed":true}}}}}},"future_metadata":{"retained":true}}`
	f.responses["/schema-download"] = []byte(`{"format_version":"1.0","provider_schemas":{"registry.terraform.io/hashicorp/aws":{"resource_schemas":{"aws_iam_role":` + selected + `,"aws_unselected":{"version":1,"block":{"description":"UNSELECTED-TYPE"}}}},"registry.terraform.io/hashicorp/other":{"resource_schemas":{"other_resource":{"block":{"description":"UNSELECTED-PROVIDER"}}}}}}`)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	encoded, err := json.Marshal(out.Types[0].ManagedSchema)
	require.NoError(t, err)
	assert.JSONEq(t, selected, string(encoded))
	packet, _ := json.Marshal(out)
	assert.NotContains(t, string(packet), "UNSELECTED-")
}

func TestPrepareImportToolAcceptsDeepUnselectedProviderSchema(t *testing.T) {
	f := importBackendFixture(t)
	var artifact map[string]any
	require.NoError(t, json.Unmarshal(f.responses["/schema-download"], &artifact))
	resources := artifact["provider_schemas"].(map[string]any)["registry.terraform.io/hashicorp/aws"].(map[string]any)["resource_schemas"].(map[string]any)
	var nested any = "unselected"
	for range 45 {
		nested = map[string]any{"block_types": nested}
	}
	resources["aws_unselected_deep"] = map[string]any{"version": 1, "block": nested}
	var err error
	f.responses["/schema-download"], err = json.Marshal(artifact)
	require.NoError(t, err)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	encoded, _ := json.Marshal(out)
	assert.NotContains(t, string(encoded), "unselected")
}

func TestPrepareImportToolWithoutReleaseMetadata(t *testing.T) {
	f := importBackendFixture(t)
	f.responses["/api/v2/search/no-code-query/ncqry-fixture"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/search/no-code-query/ncqry-fixture"]), `"version": "6.62.0",`, ""))
	f.responses["/api/v2/runs/run-schema"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/runs/run-schema"]), `"terraform-version": "1.16.1",`, ""))
	discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	input := importFixtureInput(t)
	input.Selections[0].CandidateID = discovery.Candidates[0].CandidateID
	out := prepareImportTool(context.Background(), f.client, input)
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	assert.Empty(t, out.Carry.Destination.TerraformVersion)
	assert.Equal(t, importIdentitySupported, out.Types[0].IdentitySupport, "an identity schema is support regardless of version")
}

func TestPrepareImportToolIndependentOfCreateEligibility(t *testing.T) {
	for _, mode := range []string{"remote", "agent", "local"} {
		t.Run(mode, func(t *testing.T) {
			f := importBackendFixture(t)
			path := "/api/v2/organizations/fixture-org/workspaces/import-root"
			var wire map[string]any
			require.NoError(t, json.Unmarshal(f.responses[path], &wire))
			data := wire["data"].(map[string]any)
			attrs := data["attributes"].(map[string]any)
			attrs["execution-mode"] = mode
			attrs["working-directory"] = "environments/production"
			attrs["vcs-repo"] = map[string]any{"identifier": "example/configuration"}
			delete(data["relationships"].(map[string]any), "current-configuration-version")
			var err error
			f.responses[path], err = json.Marshal(wire)
			require.NoError(t, err)
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			require.Equal(t, "prepared", out.Status, out.Diagnostics)
			assert.Equal(t, mode, out.ExecutionMode)
			assert.Equal(t, "environments/production", out.Baseline.WorkingDirectory)
			assert.False(t, out.HasCurrentConfiguration)
		})
	}
}

func TestPrepareImportToolBlankWorkspace(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "ready_for_authoring", out.Status, out.Diagnostics)
	assert.False(t, out.HasCurrentConfiguration)
	assert.Equal(t, importIdentityUnknown, out.Carry.Destination.IdentitySupport["aws_iam_role"])
	assert.Equal(t, "unknown", out.Types[0].ManagedTypeSupport)
	assert.Contains(t, out.NextAction, "no current configuration")
	f.stateStatus = http.StatusForbidden
	blocked := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", blocked.Status, "inaccessible state is not an empty workspace")
}

func TestPrepareImportToolResponseLimitNeverTruncates(t *testing.T) {
	f := importBackendFixture(t)
	big := strings.Repeat("x", maxImportPreparationBytes)
	f.responses["/schema-download"] = []byte(`{"format_version":"1.0","provider_schemas":{"registry.terraform.io/hashicorp/aws":{"resource_schemas":{"aws_iam_role":{"version":1,"block":{"description":"` + big + `"}}}}}}`)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "evidence_response_limit")
	assert.Nil(t, out.Carry)
	assert.Empty(t, out.Types)
}

func TestDownloadToolFailuresReturnNoURL(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*importBackendTest)
	}{
		{"missing CV", "configuration_source_unavailable", func(f *importBackendTest) {
			f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"] = []byte(strings.Replace(string(f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"]), `"current-configuration-version": {"data": {"type": "configuration-versions", "id": "cv-current"}},`, "", 1))
		}},
		{"unuploaded CV", "configuration_source_not_uploaded", func(f *importBackendTest) {
			f.responses["/api/v2/configuration-versions/cv-current"] = []byte(strings.Replace(string(f.responses["/api/v2/configuration-versions/cv-current"]), `"status": "uploaded"`, `"status": "pending"`, 1))
		}},
		{"missing artifact", "backend_evidence_unavailable", func(f *importBackendTest) { f.cvDownloadStatus = http.StatusNotFound }},
		{"no redirect", "configuration_download_location_unavailable", func(f *importBackendTest) { f.cvDownloadStatus = http.StatusOK }},
		{"changed baseline", "baseline_changed", func(f *importBackendTest) { f.baselineChanged = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := importBackendFixture(t)
			t.Setenv(client.TerraformAddress, f.url)
			t.Setenv(client.TerraformToken, "fixture-token")
			tc.change(f)
			out := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root"}, "cv-current", silentLogger())
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, tc.code)
			assert.Empty(t, out.DownloadURL)
			f.mu.Lock()
			assert.Equal(t, 0, f.requests["GET /cv-archive"])
			f.mu.Unlock()
		})
	}
}

func TestDownloadToolBlankWorkspaceDoesNotClaimUnobservedState(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	out := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root"}, "cv-current", silentLogger())
	assert.Equal(t, "blank_workspace", out.Status)
	assert.Empty(t, out.DownloadURL)
	f.stateStatus = http.StatusForbidden
	blocked := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root"}, "cv-current", silentLogger())
	assert.Equal(t, "blocked", blocked.Status)
}

func TestImportIdentityComparison(t *testing.T) {
	source := map[string]any{"account_id": "123456789012", "name": "role-a"}
	for _, tc := range []struct{ name, after, status string }{
		{"provider returned same identity", `{"name":"role-a","account_id":"123456789012"}`, "matched"},
		{"provider returned different object", `{"account_id":"123456789012","name":"role-b"}`, "mismatched"},
		{"only authored import ID", `null`, "unverified"},
		{"provider returned extra identity key", `{"account_id":"123456789012","name":"role-a","region":"us-east-1"}`, "unverified"},
		{"unknown numeric semantics", `{"account_id":"123456789012","name":1}`, "unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := importCompareProviderIdentity(source, json.RawMessage(tc.after))
			assert.Equal(t, tc.status, status)
		})
	}
}

func TestImportToolsMCPExchange(t *testing.T) {
	for _, transport := range []string{"in_process", "streamable_http"} {
		t.Run(transport, func(t *testing.T) {
			f := importBackendFixture(t)
			t.Setenv(client.TerraformAddress, f.url)
			t.Setenv(client.TerraformToken, "fixture-token")
			logger := silentLogger()
			s := server.NewMCPServer("import-test", "1", server.WithToolCapabilities(true), server.WithToolHandlerMiddleware(client.ToolLoggingMiddleware(logger)), server.WithToolHandlerMiddleware(client.OrganizationAllowlistToolMiddleware([]string{"fixture-org"}, logger)))
			wrap := func(h func(context.Context, mcp.CallToolRequest, *log.Logger) (*mcp.CallToolResult, error)) server.ToolHandlerFunc {
				return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return h(ctx, r, logger)
				}
			}
			s.AddTool(PrepareImportDefinition(), wrap(HandlePrepareImport))
			s.AddTool(GetImportConfigurationDownloadDefinition(), wrap(HandleGetImportConfigurationDownload))
			s.AddTool(CreateImportCVDefinition(), wrap(HandleCreateImportCV))
			s.AddTool(CreateImportRunDefinition(), wrap(HandleCreateImportRun))
			s.AddTool(VerifyImportPlanDefinition(), wrap(HandleVerifyImportPlan))

			var c *mcpclient.Client
			var err error
			if transport == "in_process" {
				c, err = mcpclient.NewInProcessClient(s)
			} else {
				h := server.NewTestStreamableHTTPServer(s)
				t.Cleanup(h.Close)
				c, err = mcpclient.NewStreamableHttpClient(h.URL)
			}
			require.NoError(t, err)
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, c.Start(ctx))
			_, err = c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "text-only-test", Version: "1"}}})
			require.NoError(t, err)
			tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
			require.NoError(t, err)
			require.Len(t, tools.Tools, 5)
			result, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "prepare_import", Arguments: map[string]any{
				"organization_name": "fixture-org", "workspace_name": "import-root", "query_run_id": "qry-fixture",
				"selections": []any{map[string]any{"candidate_id": importFixtureInput(t).Selections[0].CandidateID, "managed_type": "aws_iam_role"}},
			}}})
			require.NoError(t, err)
			require.False(t, result.IsError)
			text, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			var response importPrepared
			require.NoError(t, json.Unmarshal([]byte(text.Text), &response))
			assert.Equal(t, "prepared", response.Status, response.Diagnostics)
			require.NotNil(t, response.Carry)
			assert.Equal(t, importCarryDigest(*response.Carry), response.Carry.SelectionDigest, "the carry block survives a text-only round trip")
			assert.NotNil(t, result.StructuredContent)
		})
	}
}
