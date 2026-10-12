// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchRegistrationAndConfigurationCall(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "search-test", Version: "0.0.0"}, nil)
	RegisterTools(srv, slog.Default(), toolsets.NewToolsetFilter([]string{toolsets.Search}))
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "search-test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	want := map[string]bool{
		"generate_query_configuration": false, "provider_list_schema_list": false,
		"execute_query": false, "get_query_status": false, "get_query_summary": false,
	}
	require.Len(t, listed.Tools, len(want), "search-only filter must not expose other tools")
	for _, tool := range listed.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
			require.NotNil(t, tool.InputSchema)
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
	ctx := context.Background()
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
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "search-test", Version: "0.0.0"}, nil)
	RegisterTools(srv, slog.Default(), toolsets.NewToolsetFilter([]string{toolsets.Search}))
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "search-test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"organization_name": "org", "workspace_name": "ws"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "catalog result: %#v", result.Content)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "supported_providers")
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"organization_name": "org", "workspace_name": "not-found"}})
	require.NoError(t, err)
	require.True(t, result.IsError, "missing workspace must not be treated as authorized")

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "execute_query", Arguments: map[string]any{
		"organization_name": "org", "workspace_name": "ws",
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
