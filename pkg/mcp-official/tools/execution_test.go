// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const executionConfiguration = `{"no_code_query_providers":[{"namespace":"hashicorp","name":"example","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"example_list"}}]}]}`

func TestOfficialExecutionAndStatusWireResults(t *testing.T) {
	var submissions, workspaceReads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch {
		case r.URL.Path == "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/organizations/org/workspaces/ws":
			workspaceReads.Add(1)
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/search/no-code-query":
			submissions.Add(1)
			var body struct {
				Data struct {
					Attributes    map[string]any `json:"attributes"`
					Relationships map[string]struct {
						Data struct {
							ID string `json:"id"`
						} `json:"data"`
					} `json:"relationships"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.Data.Attributes["source"] != "terraform-mcp" || body.Data.Relationships["workspace"].Data.ID != "ws-test" {
				t.Error("query submission must use the verified source workspace")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"data":{"type":"no-code-queries","id":"ncqry-test","artifact_url":"https://private.invalid/secret-sentinel","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-finished"}}}}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/queries/qry-"):
			status := strings.TrimPrefix(r.URL.Path, "/api/v2/queries/qry-")
			if status != "finished" && status != "errored" && status != "canceled" {
				http.Error(w, "backend-secret-sentinel", http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-%s","attributes":{"status":%q,"log-read-url":"https://private.invalid/secret-sentinel"}}}`, status, status)
		default:
			t.Errorf("unexpected backend access: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	ctx, session := newOfficialSearchSession(t)
	for _, invalid := range []string{" ", "{", "{}"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "execute_query", Arguments: map[string]any{
			"terraform_org_name": "org", "workspace_name": "ws", "query_configuration": invalid,
		}})
		require.NoError(t, err)
		require.True(t, result.IsError)
	}
	require.Zero(t, workspaceReads.Load(), "validation must precede scoped access")
	require.Zero(t, submissions.Load())
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "execute_query", Arguments: map[string]any{
		"terraform_org_name": "org", "workspace_name": "ws", "query_configuration": executionConfiguration,
	}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, int32(1), submissions.Load())
	require.Equal(t, map[string]any{"query_run_id": "qry-finished"}, result.StructuredContent)
	require.JSONEq(t, `{"query_run_id":"qry-finished"}`, result.Content[0].(*mcp.TextContent).Text)
	for _, status := range []string{"finished", "errored", "canceled"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_status", Arguments: map[string]any{"query_run_id": "qry-" + status}})
		require.NoError(t, err)
		require.False(t, result.IsError, "terminal failure is a factual status")
		structured := result.StructuredContent.(map[string]any)
		require.Equal(t, status, structured["status"])
		require.Equal(t, "qry-"+status, structured["query_run_id"])
		require.Equal(t, true, structured["terminal"])
		text := result.Content[0].(*mcp.TextContent).Text
		_, fallback, ok := strings.Cut(text, "\n\n")
		require.True(t, ok)
		encoded, err := json.Marshal(structured)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), fallback)
		require.NotContains(t, text, "secret-sentinel")
		require.NotContains(t, string(encoded), "log-read-url")
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_status", Arguments: map[string]any{"query_run_id": "qry-missing"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Nil(t, result.StructuredContent)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "query_run_unavailable")
	require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, "secret-sentinel")
	require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, backend.URL)
	require.Equal(t, int32(1), submissions.Load(), "monitoring must never resubmit")
}

func TestOfficialExecutionUnknownOutcomeDoesNotRetry(t *testing.T) {
	for _, response := range []string{"error", "{}", "{"} {
		t.Run(response, func(t *testing.T) {
			var submissions atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/organizations/org/workspaces/ws":
					_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test"}}`)
				case "/api/v2/search/no-code-query":
					submissions.Add(1)
					if response == "error" {
						http.Error(w, "backend-secret-sentinel", http.StatusBadRequest)
						return
					}
					_, _ = fmt.Fprint(w, response)
				default:
					t.Errorf("unexpected backend access: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			ctx, session := newOfficialSearchSession(t)
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "execute_query", Arguments: map[string]any{
				"terraform_org_name": "org", "workspace_name": "ws", "query_configuration": executionConfiguration,
			}})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "outcome unknown; reconcile")
			require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, "secret-sentinel")
			require.Equal(t, int32(1), submissions.Load())
		})
	}
}
