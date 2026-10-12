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

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterToolsSelectsCreateRunVariant(t *testing.T) {
	tests := []struct {
		name            string
		tfOpsEnabled    string
		wantDestructive bool
		wantRunTypes    []any
	}{
		{
			name:            "safe",
			tfOpsEnabled:    "false",
			wantDestructive: false,
			wantRunTypes:    []any{"plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply"},
		},
		{
			name:            "all run types",
			tfOpsEnabled:    "true",
			wantDestructive: true,
			wantRunTypes:    []any{"plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply", "auto_approve", "is_destroy"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ENABLE_TF_OPERATIONS", test.tfOpsEnabled)

			implementation := &mcp.Implementation{Name: "test", Version: "test"}
			server := mcp.NewServer(implementation, nil)
			RegisterTools(
				server,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				toolsets.NewIndividualToolFilter([]string{"create_run"}),
			)

			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, serverSession.Close()) })

			clientSession, err := mcp.NewClient(implementation, nil).Connect(t.Context(), clientTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, clientSession.Close()) })

			result, err := clientSession.ListTools(t.Context(), nil)
			require.NoError(t, err)
			require.Len(t, result.Tools, 1)

			tool := result.Tools[0]
			assert.Equal(t, "create_run", tool.Name)
			require.NotNil(t, tool.Annotations)
			require.NotNil(t, tool.Annotations.DestructiveHint)
			assert.Equal(t, test.wantDestructive, *tool.Annotations.DestructiveHint)

			data, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			var schema jsonschema.Schema
			require.NoError(t, json.Unmarshal(data, &schema))
			assert.Equal(t, test.wantRunTypes, schema.Properties["run_type"].Enum)
		})
	}
}

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
		if r.URL.Path != "/api/v2/queries/qry-test" {
			http.Error(w, "unexpected backend path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
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
			_, _ = fmt.Fprint(w, `{"data":{"type":"no-code-queries","id":"ncqry-test","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-test"}}}}}`)
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
}
