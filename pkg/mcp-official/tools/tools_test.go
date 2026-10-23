// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	legacymcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func newOfficialSearchSession(t *testing.T, middlewares ...mcp.Middleware) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "search-test", Version: "0"}, nil)
	srv.AddReceivingMiddleware(middlewares...)
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
	require.Len(t, listed.Tools, 5, "search-only filter must expose discovery, guidance, execution, status, and summary")
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		require.NotNil(t, tool.InputSchema)
		if tool.Name == "execute_query" || tool.Name == "get_query_status" || tool.Name == "get_query_summary" {
			require.NotNil(t, tool.OutputSchema)
		} else {
			require.Nil(t, tool.OutputSchema, "retain the text result contracts")
		}
		require.Equal(t, tool.Name != "execute_query", tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
	}
	require.ElementsMatch(t, []string{"provider_list_schema_list", "generate_query_configuration", "execute_query", "get_query_status", "get_query_summary"}, names)

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
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "generate_query_configuration", Arguments: map[string]any{
		"list_resource_schemas": string(fetched.Schemas), "provider_name": fetched.Name,
		"provider_namespace": fetched.Namespace, "provider_version": fetched.Version,
	}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `"resource_type": "example_list"`)
	require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `"version": "1.0.0"`)

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

func TestSearchGeneratorTransportParityWithoutCredentials(t *testing.T) {
	for _, name := range []string{"TFE_TOKEN", "TFE_TOKEN_PATH", "TF_TOKEN_app_terraform_io", "TF_CLOUD_ORGANIZATION", "TF_WORKSPACE", "TF_MCP_QUERY_WORKSPACE"} {
		t.Setenv(name, "")
	}
	t.Setenv("TFE_ADDRESS", "http://127.0.0.1:1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TF_CLI_CONFIG_FILE", t.TempDir()+"/absent.tfrc")
	ctx, session := newOfficialSearchSession(t)
	logger := log.New()
	logger.SetOutput(io.Discard)
	legacy := searchlogic.GenerateQueryConfiguration(logger)
	const bare = `{"example_exact_list":{"block":{"attributes":{"region":{"type":"string","required":true}},"block_types":{"filter":{"nesting_mode":"list","min_items":1,"max_items":2,"block":{"attributes":{"name":{"type":"string","required":true}}}}}}}}`
	for _, test := range []struct {
		name, schema, resourceTypes string
		invalid                     bool
	}{
		{"bare", bare, "", false},
		{"wrapped", `{"list_resource_schemas":` + bare + `}`, "example_exact_list", false},
		{"malformed", "{", "", true},
		{"empty bare", "{}", "", true},
		{"empty wrapped", `{"list_resource_schemas":{}}`, "", true},
		{"unknown resource", bare, "example_managed", true},
		{"mixed known and unknown", bare, "example_exact_list,example_managed", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{
				"list_resource_schemas": test.schema, "provider_name": "Example",
				"provider_namespace": "hashicorp", "provider_version": "1.2.3",
				"resource_types": test.resourceTypes,
			}
			oldResult, err := legacy.Handler(ctx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Name: legacy.Tool.Name, Arguments: args}})
			require.NoError(t, err)
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: legacy.Tool.Name, Arguments: args})
			require.NoError(t, err)
			require.Equal(t, test.invalid, oldResult.IsError)
			require.Equal(t, test.invalid, result.IsError)
			require.Nil(t, result.StructuredContent)
			if test.invalid {
				require.NotEmpty(t, result.Content)
				return
			}
			text := result.Content[0].(*mcp.TextContent).Text
			require.Equal(t, oldResult.Content[0].(legacymcp.TextContent).Text, text)
			require.Contains(t, text, `"version": "1.2.3"`)
			require.Contains(t, text, `"resource_type": "example_exact_list"`)
			require.Contains(t, text, `"name": "example"`)
		})
	}
	for _, field := range []string{"list_resource_schemas", "provider_name", "provider_namespace", "provider_version"} {
		t.Run("missing/"+field, func(t *testing.T) {
			args := map[string]any{"list_resource_schemas": bare, "provider_name": "example", "provider_namespace": "hashicorp", "provider_version": "1.2.3"}
			delete(args, field)
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: legacy.Tool.Name, Arguments: args})
			require.NoError(t, err)
			require.True(t, result.IsError, "native schema must reject missing required fields")
		})
	}
	// The official SDK lists credential-requiring tools, but checks credentials at call time.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "provider_list_schema_list", Arguments: map[string]any{
		"terraform_org_name": "org", "workspace_name": "ws",
	}})
	require.NoError(t, err)
	require.True(t, result.IsError, "provider discovery must remain unavailable without credentials")
	require.Nil(t, result.StructuredContent)
	require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, "127.0.0.1")
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"execute_query", map[string]any{"terraform_org_name": "org", "workspace_name": "ws", "query_configuration": executionConfiguration}},
		{"get_query_status", map[string]any{"query_run_id": "qry-test"}},
		{"get_query_summary", map[string]any{"query_run_id": "qry-test"}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		require.NoError(t, err)
		require.True(t, result.IsError, "%s must require credentials at call time", test.name)
		require.Nil(t, result.StructuredContent)
		require.NotContains(t, result.Content[0].(*mcp.TextContent).Text, "127.0.0.1")
	}
}
