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
	"github.com/mark3labs/mcp-go/mcp"
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
	result, err := query.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query_run_id": "qry-fixture", "include_import_candidates": true}}})
	require.NoError(t, err)
	require.False(t, result.IsError, result.Content)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var discovery struct {
		Candidates []struct {
			CandidateID string `json:"candidate_id"`
		} `json:"candidates"`
	}
	require.NoError(t, json.Unmarshal(raw, &discovery))
	require.Len(t, discovery.Candidates, 1)
	assert.True(t, strings.HasPrefix(discovery.Candidates[0].CandidateID, "candidate-"))

	tool := ImportQueryResults(silentLogger(), nil)
	assert.Equal(t, "import_query_results", tool.Tool.Name)
	assert.Contains(t, tool.Tool.InputSchema.Properties, "selections")
	result, err = tool.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"phase": "verify", "organization_name": "fixture-org", "workspace_name": "import-root"}}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	encoded, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "legacy_import_contract")
}
