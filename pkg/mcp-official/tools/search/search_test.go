// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchArgumentsAndValidation(t *testing.T) {
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema, ok := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, statusSchema.Required, "terminal")
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")

	_, _, err = GetQueryStatusFunc(slog.Default())(context.Background(), nil, QueryRunArguments{QueryRunID: " "})
	require.ErrorContains(t, err, "missing required input: query_run_id")
	_, _, err = ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
		OrganizationName: "org", WorkspaceName: "ws", QueryConfiguration: `{`,
	})
	require.ErrorContains(t, err, "invalid query_configuration")
}

func TestOfficialSearchToolAnnotations(t *testing.T) {
	for _, tool := range []*mcp.Tool{GenerateQueryConfigurationTool(), ProviderListSchemaListTool(), GetQueryStatusTool(), GetQuerySummaryTool()} {
		require.True(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
	}
	require.False(t, ExecuteQueryTool().Annotations.ReadOnlyHint)
	require.Equal(t, searchlogic.ExecuteQueryDescription, ExecuteQueryTool().Description)
	require.Contains(t, ExecuteQueryTool().Description, "EXECUTION APPROVAL")
	require.Contains(t, ExecuteQueryTool().Description, "wait for their approval")
}

func TestOfficialSearchScopeSchemas(t *testing.T) {
	providerSchema, ok := ProviderListSchemaListTool().InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	querySchema, ok := ExecuteQueryTool().InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	for _, schema := range []*jsonschema.Schema{providerSchema, querySchema} {
		require.NotContains(t, schema.Required, "organization_name")
		require.NotContains(t, schema.Required, "workspace_name")
		require.Equal(t, searchlogic.QueryOrganizationDescription, schema.Properties["organization_name"].Description)
		require.Equal(t, searchlogic.QueryWorkspaceDescription, schema.Properties["workspace_name"].Description)
	}
	require.Equal(t, []string{"query_configuration"}, querySchema.Required)
}

func TestOfficialSearchScopeClarification(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	mcp.AddTool(server, ProviderListSchemaListTool(), ProviderListSchemaListFunc)
	mcp.AddTool(server, ExecuteQueryTool(), ExecuteQueryFunc)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	for _, test := range []struct {
		name, org, workspace, mcpWorkspace, status string
		missing                                    []string
	}{
		{"missing", "", "", "", "input_required", []string{"organization_name", "workspace_name"}},
		{"missing organization with Terraform workspace", "", "ws", "", "input_required", []string{"organization_name"}},
		{"missing workspace with Terraform organization", "org", "", "", "input_required", []string{"workspace_name"}},
		{"missing organization with MCP workspace", "", "", "query", "input_required", []string{"organization_name"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
			t.Setenv("TF_WORKSPACE", test.workspace)
			t.Setenv("TF_MCP_QUERY_WORKSPACE", test.mcpWorkspace)
			for _, name := range []string{"provider_list_schema_list", "execute_query"} {
				// Invalid configuration must not be validated until scope is resolved.
				arguments := map[string]any{}
				if name == "execute_query" {
					arguments["query_configuration"] = "{"
				}
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
				require.NoError(t, err)
				require.False(t, result.IsError)
				require.Len(t, result.Content, 1)
				text, ok := result.Content[0].(*mcp.TextContent)
				require.True(t, ok)
				var payload struct {
					Status  string   `json:"status"`
					Message string   `json:"message"`
					Missing []string `json:"missing_inputs"`
				}
				require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
				require.Equal(t, test.status, payload.Status)
				require.Equal(t, test.missing, payload.Missing)
				require.Contains(t, payload.Message, "ask the user")
			}
		})
	}
}

func TestOfficialExecuteQueryUsesEnvironmentScopeDirectly(t *testing.T) {
	for _, workspaceEnv := range []string{"TF_WORKSPACE", "TF_MCP_QUERY_WORKSPACE"} {
		t.Run(workspaceEnv, func(t *testing.T) {
			t.Setenv("TF_CLOUD_ORGANIZATION", "org")
			t.Setenv("TF_WORKSPACE", "")
			t.Setenv("TF_MCP_QUERY_WORKSPACE", "")
			t.Setenv(workspaceEnv, "workspace")
			_, _, err := ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
				QueryConfiguration: "{",
			})
			// Resolved environment scope must reach validation, not request user input.
			require.ErrorContains(t, err, "invalid query_configuration")
		})
	}
}
