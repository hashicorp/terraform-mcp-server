// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools/search/importworkflow"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchImportToolBoundary(t *testing.T) {
	fixture := filepath.Join("testdata", "import", "same-version")
	backend, err := os.ReadFile(filepath.Join(fixture, "backend.json"))
	require.NoError(t, err)
	var responses map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(backend, &responses))
	responses["/api/v2/workspaces/ws-fixture"] = responses["/api/v2/organizations/fixture-org/workspaces/import-root"]
	logData, err := os.ReadFile(filepath.Join(fixture, "query.ndjson"))
	require.NoError(t, err)
	var backendServer *httptest.Server
	backendServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			return
		case "/logs":
			data := append(append([]byte{2}, logData...), 3)
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(data) {
				_, _ = w.Write(data[offset:min(offset+limit, len(data))])
			}
			return
		case "/api/v2/queries/qry-fixture":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-fixture","attributes":{"status":"finished","generate-config-out":true,"log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-fixture"}},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-query"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-fixture"}}}}}`, backendServer.URL+"/logs")
			return
		case "/api/v2/workspaces/ws-fixture/current-state-version":
			_, _ = w.Write([]byte(`{"data":{"type":"state-versions","id":"sv-current","attributes":{"serial":42},"relationships":{"run":{"data":{"type":"runs","id":"run-schema"}}}}}`))
			return
		case "/api/v2/runs/run-schema/plan/json-schema":
			http.Redirect(w, r, backendServer.URL+"/schema-download", http.StatusTemporaryRedirect)
			return
		case "/schema-download":
			data, err := os.ReadFile(filepath.Join(fixture, "provider-schema.json"))
			require.NoError(t, err)
			_, _ = w.Write(data)
			return
		default:
			if body, ok := responses[r.URL.Path]; ok {
				_, _ = w.Write(body)
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(backendServer.Close)
	t.Setenv(client.TerraformAddress, backendServer.URL)
	t.Setenv(client.TerraformToken, "fixture-token")

	query := GetQuerySummary(silentLogger())
	result, err := query.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query_run_id": "qry-fixture"}}})
	require.NoError(t, err)
	require.False(t, result.IsError, result.Content)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var discovery struct {
		Lists []struct {
			Candidates []struct {
				CandidateID string `json:"candidate_id"`
			} `json:"candidates"`
		} `json:"lists"`
	}
	require.NoError(t, json.Unmarshal(raw, &discovery))
	require.Len(t, discovery.Lists, 1)
	require.Len(t, discovery.Lists[0].Candidates, 1)
	candidateID := discovery.Lists[0].Candidates[0].CandidateID
	assert.True(t, strings.HasPrefix(candidateID, "candidate-"))

	selection := map[string]any{"candidate_id": candidateID, "managed_type": "aws_iam_role"}
	prepare := PrepareImport(silentLogger())
	assert.Equal(t, "prepare_import", prepare.Tool.Name)
	result, err = prepare.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "query_run_id": "qry-fixture", "selections": []any{selection}}}})
	require.NoError(t, err)
	require.False(t, result.IsError, result.StructuredContent)
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"selection_digest"`)
	assert.Contains(t, string(encoded), `"identity_support":"supported"`)
}

func TestSearchImportToolDefinitions(t *testing.T) {
	logger := silentLogger()
	expected := map[string]struct {
		readOnly bool
		required []string
	}{
		"prepare_import":                    {true, []string{"organization_name", "workspace_name", "query_run_id", "selections"}},
		"get_import_configuration_download": {true, []string{"organization_name", "workspace_name", "configuration_version_id"}},
		"create_import_cv":                  {false, []string{"organization_name", "workspace_name", "confirm_speculative_run"}},
		"create_import_run":                 {false, []string{"organization_name", "workspace_name", "configuration_version_id", "confirm_speculative_run"}},
		"verify_import_plan":                {true, []string{"organization_name", "workspace_name", "run_id"}},
	}
	for _, tool := range []server.ServerTool{PrepareImport(logger), GetImportConfigurationDownload(logger), CreateImportCV(logger), CreateImportRun(logger), VerifyImportPlan(logger)} {
		want, ok := expected[tool.Tool.Name]
		require.True(t, ok, tool.Tool.Name)
		require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint, tool.Tool.Name)
		assert.Equal(t, want.readOnly, *tool.Tool.Annotations.ReadOnlyHint, tool.Tool.Name)
		for _, field := range want.required {
			assert.Contains(t, tool.Tool.InputSchema.Required, field, tool.Tool.Name)
		}
		assert.NotNil(t, tool.Tool.OutputSchema, tool.Tool.Name)
		assert.NotNil(t, tool.Handler)
	}
	assert.NotContains(t, GetQuerySummary(logger).Tool.InputSchema.Properties, "include_import_candidates")
	for _, field := range []string{"resource_type", "address", "name_contains", "limit", "after"} {
		assert.Contains(t, GetQuerySummary(logger).Tool.InputSchema.Properties, field)
	}
}

func TestQuerySummaryTextUsesSharedImportVocabulary(t *testing.T) {
	lower := strings.ToLower(GetQuerySummary(silentLogger()).Tool.Description)
	for _, phrase := range importworkflow.RetiredImportPhrases() {
		assert.NotContains(t, lower, phrase)
	}
	assert.NotContains(t, lower, "route 1")
	assert.NotContains(t, lower, "route 2")
}
