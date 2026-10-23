// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"io"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestSearchRegistrationWithoutCredentials(t *testing.T) {
	t.Setenv("TFE_TOKEN", "")
	t.Setenv("TFE_ADDRESS", "http://127.0.0.1:1")
	previous := globalToolRegistry
	t.Cleanup(func() {
		globalToolRegistry = previous
		client.SetToolRegistryCallback(previous)
	})
	logger := log.New()
	logger.SetOutput(io.Discard)
	svr := server.NewMCPServer("search-test", "0")
	RegisterTools(svr, logger, toolsets.NewToolsetFilter([]string{toolsets.Search}))
	require.Len(t, svr.ListTools(), 1)
	require.Nil(t, svr.GetTool("provider_list_schema_list"))
	require.Nil(t, svr.GetTool("execute_query"))
	require.Nil(t, svr.GetTool("get_query_status"))
	require.Nil(t, svr.GetTool("get_query_summary"))
	require.Nil(t, svr.GetTool("prepare_import"))
	generator := svr.GetTool("generate_query_configuration")
	require.NotNil(t, generator)
	result, err := generator.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name: generator.Tool.Name,
		Arguments: map[string]any{
			"list_resource_schemas": `{"example_list":{"block":{"attributes":{}}}}`,
			"provider_name":         "example", "provider_namespace": "hashicorp", "provider_version": "1.2.3",
		},
	}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.False(t, globalToolRegistry.HasAnySessionWithTFE())

	// Simulate credential availability without constructing a client or calling a backend.
	globalToolRegistry.RegisterSessionWithTFE("fixture-session")
	require.Len(t, svr.ListTools(), 6)
	for _, name := range []string{"execute_query", "get_query_status", "get_query_summary", "prepare_import"} {
		tool := svr.GetTool(name)
		require.NotNil(t, tool)
		result, err := tool.Handler(context.Background(), mcp.CallToolRequest{})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].(mcp.TextContent).Text, "requires an active session")
	}
	provider := svr.GetTool("provider_list_schema_list")
	require.NotNil(t, provider)
	result, err = provider.Handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.True(t, result.IsError, "provider discovery must still require an authenticated session")
	require.Contains(t, result.Content[0].(mcp.TextContent).Text, "requires an active session")
}

// TestToolFactoriesStayInSyncWithAllTools helps keep toolsets.AllTools and toolFactories in sync
// This test fails when someone adds a tool to registry.go and forgets to add its builder to factories.go or vice versa
func TestToolFactoriesStayInSyncWithAllTools(t *testing.T) {
	known := make(map[string]bool, len(toolsets.AllTools))
	for _, td := range toolsets.AllTools {
		known[td.Name] = true
		_, plain := toolFactories[td.Name]
		_, special := specialFactories[td.Name]
		if !plain && !special {
			t.Errorf("toolsets.AllTools has %q but no factory builds it", td.Name)

		}
	}
	for name := range toolFactories {
		if !known[name] {
			t.Errorf("toolFactories has %q but toolsets.AllTools does not know about it", name)
		}
	}
	for name := range specialFactories {
		if !known[name] {
			t.Errorf("specialFactories has %q but toolsets.AllTools does not know about it", name)
		}
	}

}
