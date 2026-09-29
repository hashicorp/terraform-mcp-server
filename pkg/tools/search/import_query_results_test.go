// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared source-backed mock material, not a live Atlas response or a server
// configuration-file handoff.
func phase0Fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata/import/same-version", name))
	require.NoError(t, err)
	return b
}

type importBackendTest struct {
	client           *tfe.Client
	url              string
	mu               sync.Mutex
	requests         map[string]int
	responses        map[string]json.RawMessage
	schemaStatus     int
	cvDownloadStatus int
	stateStatus      int
	stateChanged     bool
	deniedDownloads  int
	queryLog         []byte
	baselineChanged  bool
	mutationHandler  func(http.ResponseWriter, *http.Request) bool
}

func importBackendFixture(t *testing.T) *importBackendTest {
	t.Helper()
	f := &importBackendTest{requests: map[string]int{}, schemaStatus: 200, cvDownloadStatus: http.StatusFound, stateStatus: 200, queryLog: phase0Fixture(t, "query.ndjson")}
	require.NoError(t, json.Unmarshal(phase0Fixture(t, "backend.json"), &f.responses))
	f.responses["/api/v2/workspaces/ws-fixture"] = f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"]
	f.responses["/schema-download"] = phase0Fixture(t, "provider-schema.json")
	// Simulated state metadata: contents/download URLs must not be consumed.
	f.responses["/api/v2/workspaces/ws-fixture/current-state-version"] = json.RawMessage(`{"data":{"type":"state-versions","id":"sv-current","attributes":{"serial":42,"hosted-state-download-url":"https://must-not-download.invalid/STATE-SECRET","providers":{"provider[\"registry.terraform.io/hashicorp/aws\"]":{"aws_iam_role":1}}},"relationships":{"run":{"data":{"type":"runs","id":"run-schema"}}}}}`)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		f.requests[key]++
		if f.mutationHandler != nil && f.mutationHandler(w, r) {
			return
		}
		assert.Equal(t, http.MethodGet, r.Method, "preparation must not mutate the backend")
		if r.URL.Path == "/api/v2/ping" {
			return
		}
		if r.URL.Path == "/logs" {
			log := append([]byte{2}, f.queryLog...)
			log = append(log, 3)
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(log) {
				_, _ = w.Write(log[offset:min(offset+limit, len(log))])
			}
			return
		}
		assert.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.URL.Path == "/schema-download" {
			assert.Equal(t, "fixture", r.URL.Query().Get("signed"))
			if f.requests[key] <= f.deniedDownloads {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"errors":[{"title":"expired signed URL"}]}`)
				return
			}
		}
		if r.URL.Path == "/api/v2/runs/run-schema/plan/json-schema" {
			if f.schemaStatus == 200 {
				http.Redirect(w, r, f.url+"/schema-download?signed=fixture", http.StatusTemporaryRedirect)
			} else {
				w.WriteHeader(f.schemaStatus)
				if f.schemaStatus >= 400 {
					_, _ = io.WriteString(w, `{"errors":[{"title":"denied secret must not leak"}]}`)
				}
			}
			return
		}
		if r.URL.Path == "/api/v2/configuration-versions/cv-current/download" {
			if f.cvDownloadStatus == http.StatusFound {
				http.Redirect(w, r, f.url+"/cv-archive?signed=fixture", http.StatusFound)
			} else {
				w.WriteHeader(f.cvDownloadStatus)
			}
			return
		}
		if r.URL.Path == "/cv-archive" {
			t.Error("the MCP server must not follow the configuration archive redirect")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/current-state-version") && f.stateStatus != 200 {
			w.WriteHeader(f.stateStatus)
			_, _ = io.WriteString(w, `{"errors":[{"title":"state unavailable"}]}`)
			return
		}
		body, ok := f.responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected API call: %s", key)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/v2/queries/qry-fixture" {
			body = []byte(strings.Replace(string(body), `"generate-config-out": true`, fmt.Sprintf(`"generate-config-out": true, "log-read-url": %q`, f.url+"/logs"), 1))
		}
		if f.baselineChanged && key == "GET /api/v2/organizations/fixture-org/workspaces/import-root" && f.requests[key] > 1 {
			body = []byte(strings.ReplaceAll(string(body), "cv-current", "cv-changed"))
		}
		if f.stateChanged && strings.HasSuffix(r.URL.Path, "/current-state-version") && f.requests[key] > 1 {
			body = []byte(strings.ReplaceAll(string(body), "sv-current", "sv-changed"))
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	f.url = s.URL
	c, err := tfe.NewClient(&tfe.Config{Address: s.URL, Token: "fixture-token", HTTPClient: s.Client()})
	require.NoError(t, err)
	f.client = c
	return f
}

func importFixtureInput(t *testing.T) importPrepareInput {
	t.Helper()
	var input importPrepareInput
	require.NoError(t, json.Unmarshal([]byte(`{"phase":"prepare","organization_name":"fixture-org","workspace_name":"import-root","query_run_id":"qry-fixture","selections":[{"candidate_id":"candidate-449c792d2631e92f1dd4026529d4ce07f9256f1a3e0bb1a95c05f2432b32a40a","managed_type":"aws_iam_role"}]}`), &input))
	return input
}

func TestImportDiscoveryAcceptsLiveNoCodeResourceTypeWireName(t *testing.T) {
	f := importBackendFixture(t)
	path := "/api/v2/search/no-code-query/ncqry-fixture"
	f.responses[path] = []byte(strings.ReplaceAll(string(f.responses[path]), `"resource_type"`, `"resource-type"`))
	discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	require.Len(t, discovery.Candidates, 1)
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", discovery.Candidates[0].Provider.Source)
}

func TestImportQueryAPIOnlyPreparation(t *testing.T) {
	f := importBackendFixture(t)
	// Newer current runs and different CVs do not select the schema source.
	f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"]), "run-schema", "run-newer-unrelated"))
	f.responses["/api/v2/runs/run-schema"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/runs/run-schema"]), "cv-current", "cv-state-producing"))
	result := prepareImportFromAPIs(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "ready_for_authoring", result.Stage, result.Diagnostics)
	assert.Equal(t, "prepared", result.Status)
	assert.Equal(t, "cv-current", result.Baseline.ConfigurationVersionID)
	assert.Equal(t, "sv-current", result.Baseline.StateVersionID)
	assert.Equal(t, int64(42), result.Baseline.StateSerial)
	assert.Equal(t, "run-schema", result.SchemaSource.RunID)
	assert.Equal(t, "cv-state-producing", result.SchemaSource.ConfigurationVersionID)
	assert.Equal(t, "different_configuration_version", result.SchemaSource.ConfigurationBaselineRelation)
	assert.Equal(t, "6.62.0", result.Selection.Provider.Version)
	assert.Equal(t, "aws_iam_role", result.ManagedType)
	assert.Equal(t, "supported", result.ManagedTypeSupport)
	assert.Equal(t, "not_requested", result.SourceSchemaComparison)
	assert.Equal(t, "plan_validation_pending", result.ValidationStatus)
	assert.Empty(t, result.Diagnostics)
	assert.Equal(t, "search-import-fixture", result.Selection.ResourceObject["name"])
	assert.Empty(t, result.Selection.Configuration, "generated HCL is optional")
	managedJSON, _ := json.Marshal(result.ManagedSchema)
	identityJSON, _ := json.Marshal(result.IdentitySchema)
	assert.Contains(t, string(managedJSON), "assume_role_policy")
	assert.Contains(t, string(identityJSON), "account_id")
	toolResult, err := importPreparationResult(result)
	require.NoError(t, err)
	assert.False(t, toolResult.IsError)
	encoded, _ := json.Marshal(toolResult)
	assert.NotContains(t, string(encoded), "STATE-SECRET")
	assert.NotContains(t, string(encoded), "fixture-token")
	assert.NotContains(t, string(encoded), "hosted-state-download-url")
	assert.Contains(t, strings.Join(result.AgentInstructions, " "), "not a tool call")
	// Every attribute/nested block of the selected type survives extraction.
	var artifact struct {
		Providers map[string]struct {
			Resources map[string]json.RawMessage `json:"resource_schemas"`
		} `json:"provider_schemas"`
	}
	require.NoError(t, json.Unmarshal(f.responses["/schema-download"], &artifact))
	assert.JSONEq(t, string(artifact.Providers[result.SchemaSource.ProviderSource].Resources[result.ManagedType]), string(managedJSON))
	// Structured-aware clients must be able to validate the actual JSON objects
	// (including raw schema/identity objects) against the advertised output schema.
	schemaJSON, err := json.Marshal(ImportQueryResults(silentLogger(), nil).Tool.OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)
	var instance any
	require.NoError(t, json.Unmarshal(resultJSON, &instance))
	require.NoError(t, resolved.Validate(instance))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.requests["GET /api/v2/organizations/fixture-org/workspaces/import-root"])
	assert.Equal(t, 1, f.requests["GET /api/v2/runs/run-schema"])
	assert.Equal(t, 0, f.requests["GET /api/v2/runs/run-newer-unrelated"])
	assert.Equal(t, 2, f.requests["GET /api/v2/workspaces/ws-fixture/current-state-version"])
	assert.Equal(t, 1, f.requests["GET /api/v2/runs/run-schema/plan/json-schema"])
	assert.Equal(t, 1, f.requests["GET /schema-download"])
	for key := range f.requests {
		assert.True(t, strings.HasPrefix(key, "GET "))
		assert.NotContains(t, key, "/download")
		assert.NotContains(t, key, "/state-versions")
		assert.NotContains(t, key, "/json-output")
		assert.NotContains(t, key, "/configuration-versions/")
		assert.NotContains(t, key, "/explorer")
	}
}

func TestImportQueryAPIFailures(t *testing.T) {
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
			result := prepareImportFromAPIs(context.Background(), f.client, input)
			assert.Equal(t, "blocked", result.Status)
			assert.Contains(t, result.Diagnostics, tc.code)
			if tc.code == "managed_schema_type_missing" {
				assert.Equal(t, "unsupported", result.ManagedTypeSupport)
			}
			encoded, _ := json.Marshal(result)
			assert.NotContains(t, string(encoded), "secret must not leak")
		})
	}
}

func TestImportQueryResultsDefinitionAndLegacyMigration(t *testing.T) {
	tool := ImportQueryResults(silentLogger(), nil)
	assert.Equal(t, "import_query_results", tool.Tool.Name)
	assert.Contains(t, tool.Tool.Description, "never reads or parses HCL")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *tool.Tool.Annotations.ReadOnlyHint, "the tool can create speculative CVs and runs")
	require.NotNil(t, tool.Tool.Annotations.IdempotentHint)
	assert.False(t, *tool.Tool.Annotations.IdempotentHint, "create requests require scoped recovery")
	inputSchema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	assert.NotContains(t, string(inputSchema), `"bootstrap_upload"`)
	assert.NotContains(t, string(inputSchema), `"bootstrap_plan"`)
	for _, args := range []map[string]any{
		{"phase": "verify", "confirm_speculative_run": true, "configuration_path": "/must-not-be-read"},
		{"phase": "review", "generated_configuration": "resource \"aws_iam_role\" \"x\" {}", "output_file": "imports.tf"},
		{"phase": "prepare", "max_resources": 1},
		{"phase": "upload", "review_id": "fixture", "confirm_speculative_run": true},
		{"phase": "prepare", "selections": "not-an-array"},
	} {
		result, err := tool.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.NotNil(t, result.StructuredContent)
		data, _ := json.Marshal(result)
		assert.NotContains(t, string(data), "No active session")
		assert.NotContains(t, string(data), "elicitation")
	}
}

func TestImportQueryHandlerStatelessSDK(t *testing.T) {
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	input := importFixtureInput(t)
	raw, _ := json.Marshal(input)
	var args map[string]any
	require.NoError(t, json.Unmarshal(raw, &args))
	tool := ImportQueryResults(silentLogger(), nil)
	handler := client.ToolLoggingMiddleware(silentLogger())(client.OrganizationAllowlistToolMiddleware([]string{"fixture-org"}, silentLogger())(tool.Handler))
	result, err := handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: tool.Tool.Name, Arguments: args}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	encoded, _ := json.Marshal(result.StructuredContent)
	assert.Contains(t, string(encoded), "ready_for_authoring")
	denied := client.OrganizationAllowlistToolMiddleware([]string{"other"}, nil)(tool.Handler)
	f.mu.Lock()
	before := len(f.requests)
	f.mu.Unlock()
	result, err = denied(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	f.mu.Lock()
	assert.Len(t, f.requests, before)
	f.mu.Unlock()
}

func TestImportDiscoveryExactSelection(t *testing.T) {
	providers := map[string]workspaceProvider{"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"}}
	data := phase0Fixture(t, "query.ndjson")
	items, err := parseImportDiscovery(data, "qry-fixture", providers)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, importFixtureInput(t).Selections[0].CandidateID, items[0].CandidateID)
	for _, raw := range [][]byte{
		[]byte(strings.Split(string(data), "\n")[0]),
		append(append([]byte{}, data...), []byte(strings.Split(string(data), "\n")[0])...),
		append(append([]byte{}, data...), []byte(`{"type":"diagnostic","diagnostic":{"severity":"error"}}`)...),
		append(append([]byte{}, data...), []byte(`{"type":"list_resource_found",`)...),
	} {
		_, err := parseImportDiscovery(raw, "qry-fixture", providers)
		assert.Error(t, err)
	}
	providers["aws_iam_role"] = workspaceProvider{Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.63.0"}
	changed, err := parseImportDiscovery(data, "qry-fixture", providers)
	require.NoError(t, err)
	assert.NotEqual(t, items[0].CandidateID, changed[0].CandidateID)
}

func TestImportSDKBoundedResponse(t *testing.T) {
	f := importBackendFixture(t)
	_, _, err := readImportBackendJSON(context.Background(), f.client, "runs/run-schema/plan/json-schema", 8)
	assert.ErrorContains(t, err, "evidence_size_limit")
}

func TestImportSchemaPresignedDownloadRecovery(t *testing.T) {
	for _, failures := range []int{1, 2} {
		t.Run(strconv.Itoa(failures), func(t *testing.T) {
			f := importBackendFixture(t)
			f.deniedDownloads = failures
			result := prepareImportFromAPIs(context.Background(), f.client, importFixtureInput(t))
			if failures == 1 {
				assert.Equal(t, "prepared", result.Status, result.Diagnostics)
			} else {
				assert.Equal(t, "blocked", result.Status)
				assert.Contains(t, result.Diagnostics, "schema_source_access_denied")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			assert.Equal(t, 2, f.requests["GET /api/v2/runs/run-schema/plan/json-schema"], "reacquire at most once through the API")
			assert.Equal(t, 2, f.requests["GET /schema-download"])
		})
	}
}

func TestImportPreparationWithoutReleaseMetadata(t *testing.T) {
	f := importBackendFixture(t)
	f.responses["/api/v2/search/no-code-query/ncqry-fixture"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/search/no-code-query/ncqry-fixture"]), `"version": "6.62.0",`, ""))
	f.responses["/api/v2/runs/run-schema"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/runs/run-schema"]), `"terraform-version": "1.16.1",`, ""))
	discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	require.Len(t, discovery.Candidates, 1)
	input := importFixtureInput(t)
	input.Selections[0].CandidateID = discovery.Candidates[0].CandidateID
	result := prepareImportFromAPIs(context.Background(), f.client, input)
	require.Equal(t, "prepared", result.Status, result.Diagnostics)
	assert.Empty(t, result.Selection.Provider.Version)
	assert.Empty(t, result.SchemaSource.TerraformVersion)
	assert.Equal(t, "not_requested", result.SourceSchemaComparison)
}

func TestImportPreparationReturnsCompleteSelectedSchema(t *testing.T) {
	f := importBackendFixture(t)
	// Fixture-only schema: include a nested block and an unfamiliar future field.
	// Selection must preserve both without interpreting or converting them.
	selected := `{"version":99,"block":{"attributes":{"name":{"type":"string","required":true}},"block_types":{"settings":{"nesting_mode":"list","min_items":1,"max_items":2,"block":{"attributes":{"values":{"type":["map","string"],"optional":true,"computed":true}}}}}},"future_metadata":{"retained":true}}`
	f.responses["/schema-download"] = []byte(`{"format_version":"1.0","provider_schemas":{"registry.terraform.io/hashicorp/aws":{"resource_schemas":{"aws_iam_role":` + selected + `,"aws_unselected":{"version":1,"block":{"description":"UNSELECTED-TYPE"}}},"data_source_schemas":{"aws_data":{"block":{"description":"UNSELECTED-DATA"}}}},"registry.terraform.io/hashicorp/other":{"resource_schemas":{"other_resource":{"block":{"description":"UNSELECTED-PROVIDER"}}}}}}`)
	f.queryLog = []byte(strings.Replace(string(f.queryLog), `"resource_object":`, `"configuration":"resource \"aws_iam_role\" \"selected\" {}","import_configuration":"import { to = aws_iam_role.selected id = \"example\" }","resource_object":`, 1))
	result := prepareImportFromAPIs(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", result.Status, result.Diagnostics)
	encoded, err := json.Marshal(result.ManagedSchema)
	require.NoError(t, err)
	assert.JSONEq(t, selected, string(encoded), "schema revision differences are not compatibility failures")
	packet, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(packet), "UNSELECTED-")
	assert.NotEmpty(t, result.Selection.Configuration)
	assert.NotEmpty(t, result.Selection.ImportConfig)
	assert.Contains(t, result.EvidenceStatus, "configuration_not_validated")
	// Optional generated HCL and observation enrichment do not change candidate IDs.
	assert.Equal(t, importFixtureInput(t).Selections[0].CandidateID, result.Selection.CandidateID)
}

func TestImportSchemaPreparationIndependentOfUploadEligibility(t *testing.T) {
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
			result := prepareImportFromAPIs(context.Background(), f.client, importFixtureInput(t))
			require.Equal(t, "prepared", result.Status, result.Diagnostics)
			assert.Equal(t, mode, result.ExecutionMode)
			assert.Equal(t, "environments/production", result.Baseline.WorkingDirectory)
			assert.Contains(t, result.Notes, "configuration_baseline_unavailable")
			assert.Equal(t, "unknown", result.SchemaSource.ConfigurationBaselineRelation)
			assert.Equal(t, "plan_validation_pending", result.ValidationStatus)
		})
	}
}

func TestImportPreparationResponseLimitDoesNotTruncateSchema(t *testing.T) {
	result, err := importPreparationResult(importPreparation{
		ContractVersion: importPreparationContractVersion, Status: "prepared",
		ManagedSchema: map[string]any{"large": strings.Repeat("x", maxImportPreparationBytes)},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	response, ok := result.StructuredContent.(importPreparation)
	require.True(t, ok)
	assert.Contains(t, response.Diagnostics, "evidence_response_limit")
	assert.Nil(t, response.ManagedSchema)
}

func TestImportPreparationMCPExchange(t *testing.T) {
	for _, transport := range []string{"in_process", "streamable_http"} {
		t.Run(transport, func(t *testing.T) {
			f := importBackendFixture(t)
			t.Setenv(client.TerraformAddress, f.url)
			t.Setenv(client.TerraformToken, "fixture-token")
			t.Setenv("ENABLE_TF_OPERATIONS", "false")
			logger := silentLogger()
			s := server.NewMCPServer("import-test", "1", server.WithToolCapabilities(true), server.WithToolHandlerMiddleware(client.ToolLoggingMiddleware(logger)), server.WithToolHandlerMiddleware(client.OrganizationAllowlistToolMiddleware([]string{"fixture-org"}, logger)))
			for _, tool := range []server.ServerTool{GetQuerySummary(logger), ImportQueryResults(logger, nil)} {
				s.AddTool(tool.Tool, tool.Handler)
			}
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
			require.Len(t, tools.Tools, 2)
			discoveryResult, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "get_query_summary", Arguments: map[string]any{"query_run_id": "qry-fixture", "include_import_candidates": true}}})
			require.NoError(t, err)
			require.False(t, discoveryResult.IsError, discoveryResult.Content)
			text, ok := discoveryResult.Content[0].(mcp.TextContent)
			require.True(t, ok)
			var discovery importDiscovery
			require.NoError(t, json.Unmarshal([]byte(text.Text), &discovery))
			require.Len(t, discovery.Candidates, 1)
			result, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "import_query_results", Arguments: map[string]any{
				"phase": "prepare", "organization_name": "fixture-org", "workspace_name": "import-root", "query_run_id": "qry-fixture",
				"selections": []any{map[string]any{"candidate_id": discovery.Candidates[0].CandidateID, "managed_type": "aws_iam_role"}},
			}}})
			require.NoError(t, err)
			require.False(t, result.IsError)
			text, ok = result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			var response importPreparation
			require.NoError(t, json.Unmarshal([]byte(text.Text), &response))
			assert.Equal(t, "ready_for_authoring", response.Stage, response.Diagnostics)
			assert.Equal(t, "plan_validation_pending", response.ValidationStatus)
			assert.Equal(t, "cv-current", response.Baseline.ConfigurationVersionID)
			assert.NotNil(t, result.StructuredContent)
		})
	}
}
