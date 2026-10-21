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

func TestOfficialSearchRegistration(t *testing.T) {
	ctx, session := newOfficialSearchSession(t)
	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1, "search-only filter must expose only provider discovery")
	tool := listed.Tools[0]
	require.Equal(t, "provider_list_schema_list", tool.Name)
	require.NotNil(t, tool.InputSchema)
	require.Nil(t, tool.OutputSchema, "retain the JSON-text provider result contract")
	require.True(t, tool.Annotations.ReadOnlyHint)
	require.False(t, *tool.Annotations.DestructiveHint)

	// The Search addition must not replace refreshed-main Runs factories.
	for _, name := range []string{"list_runs", "get_run_details", "get_run_comments", "action_run"} {
		require.Contains(t, officialFactories, name)
	}
}

func TestOfficialSearchScopedCatalog(t *testing.T) {
	var workspaceReads, catalogReads atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/ws":
			workspaceReads.Add(1)
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`)
		case "/api/v2/search/provider-versions":
			if r.URL.Query().Get("filter[organization][name]") != "org" || workspaceReads.Load() == 0 {
				http.Error(w, "catalog request missing verified scope", http.StatusBadRequest)
				return
			}
			catalogReads.Add(1)
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":[{"type":"provider-versions","id":"hashicorp/example/1.0.0","attributes":{"namespace":"hashicorp","name":"example","version":"1.0.0"}}]}`)
		case "/api/v2/search/provider-versions/hashicorp/example/1.0.0":
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprint(w, `{"data":{"type":"search-compatible-provider-versions","id":"hashicorp/example/1.0.0","attributes":{"list-resource-schemas":{"example_list":{"block":{"attributes":{}}}}}}}`)
		default:
			http.Error(w, "backend-private-sentinel", http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "")
	ctx, session := newOfficialSearchSession(t)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "ws"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "catalog result: %#v", result.Content)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "supported_providers")
	require.Nil(t, result.StructuredContent)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{"terraform_org_name": "org", "workspace_name": "ws", "provider_namespace": "hashicorp", "provider_name": "example"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "schema detail without provider metadata: %#v", result.Content)
	var fetched struct {
		Namespace string          `json:"namespace"`
		Name      string          `json:"name"`
		Version   string          `json:"version"`
		Schemas   json.RawMessage `json:"list_resource_schemas"`
		Org       string          `json:"terraform_org_name"`
		Workspace string          `json:"workspace_name"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &fetched))
	require.Equal(t, "hashicorp", fetched.Namespace)
	require.Equal(t, "example", fetched.Name)
	require.Equal(t, "1.0.0", fetched.Version)
	require.Equal(t, "org", fetched.Org)
	require.Equal(t, "ws", fetched.Workspace)
	require.JSONEq(t, `{"example_list":{"block":{"attributes":{}}}}`, string(fetched.Schemas))

	before := catalogReads.Load()
	for _, args := range []map[string]any{
		{"terraform_org_name": "org", "workspace_name": "not-found"},
		{"terraform_org_name": "org", "workspace_name": "ws", "provider_namespace": "hashicorp"},
		{"terraform_org_name": "org", "workspace_name": "ws", "provider_name": "example"},
	} {
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: args})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Nil(t, result.StructuredContent)
		require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, "backend-private-sentinel")
		require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, backend.URL)
		require.Equal(t, before, catalogReads.Load(), "invalid scope/provider pair must not access catalog")
	}
}
