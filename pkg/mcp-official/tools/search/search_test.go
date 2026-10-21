// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchDescriptionsUseSharedValues(t *testing.T) {
	require.Equal(t, searchlogic.ProviderListSchemaListDescription, ProviderListSchemaListTool().Description)
	require.Equal(t, searchlogic.GenerateQueryConfigurationDescription, GenerateQueryConfigurationTool().Description)
	require.Equal(t, searchlogic.ExecuteQueryDescription, ExecuteQueryTool().Description)
	require.Equal(t, searchlogic.GetQueryStatusDescription, GetQueryStatusTool().Description)
}

func TestOfficialSearchToolAnnotations(t *testing.T) {
	tool := ProviderListSchemaListTool()
	require.True(t, tool.Annotations.ReadOnlyHint)
	require.False(t, *tool.Annotations.DestructiveHint)
	require.True(t, *tool.Annotations.OpenWorldHint)
	require.Nil(t, tool.OutputSchema, "provider discovery retains its JSON-text result contract")
	generator := GenerateQueryConfigurationTool()
	require.True(t, generator.Annotations.ReadOnlyHint)
	require.False(t, *generator.Annotations.DestructiveHint)
	require.False(t, *generator.Annotations.OpenWorldHint)
	require.Nil(t, generator.OutputSchema, "guidance retains its text result contract")
	for _, tool := range []*mcp.Tool{ExecuteQueryTool(), GetQueryStatusTool()} {
		require.Equal(t, tool.Name != "execute_query", tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
		require.True(t, *tool.Annotations.OpenWorldHint)
		require.NotNil(t, tool.OutputSchema)
	}
	require.Contains(t, ExecuteQueryTool().Description, "EXECUTION APPROVAL")
	require.Contains(t, ExecuteQueryTool().Description, "wait for their approval")
}

func TestOfficialExecutionArgumentsAndValidation(t *testing.T) {
	querySchema := ExecuteQueryTool().InputSchema.(*jsonschema.Schema)
	require.Equal(t, []string{"query_configuration"}, querySchema.Required)
	require.Len(t, querySchema.Properties, 3)
	require.Equal(t, searchlogic.QueryOrganizationDescription, querySchema.Properties[searchlogic.ScopeOrganizationInput].Description)
	require.Equal(t, searchlogic.QueryWorkspaceDescription, querySchema.Properties[searchlogic.ScopeWorkspaceInput].Description)
	require.NotContains(t, querySchema.Properties, "organization_name")
	output := ExecuteQueryTool().OutputSchema.(*jsonschema.Schema)
	require.Equal(t, []string{"query_run_id"}, output.Required)
	require.Len(t, output.Properties, 1)
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.ElementsMatch(t, []string{"query_run_id", "status", "terminal", "message"}, statusSchema.Required)
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")
	_, _, err = GetQueryStatusFunc(slog.Default())(context.Background(), nil, QueryRunArguments{QueryRunID: " "})
	require.ErrorContains(t, err, "missing required input: query_run_id")
	for _, raw := range []string{" ", "{", "{}"} {
		_, _, err = ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
			OrganizationName: "org", WorkspaceName: "ws", QueryConfiguration: raw,
		})
		require.ErrorContains(t, err, "query_configuration")
	}
}

func TestOfficialExecuteQueryValidatesAfterAcceptedScope(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{InputResponses: mcp.InputResponseMap{
		searchlogic.ScopeInputRequestID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{
			searchlogic.ScopeOrganizationInput: "org",
			searchlogic.ScopeWorkspaceInput:    "workspace",
		}},
	}}}
	_, _, err := ExecuteQueryFunc(context.Background(), request, ExecuteQueryArguments{QueryConfiguration: "{"})
	require.ErrorContains(t, err, "invalid query_configuration")
}

func TestOfficialExecuteQueryRejectsDisallowedSourceBeforeClientUse(t *testing.T) {
	ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed"})
	result, output, err := ExecuteQueryFunc(ctx, nil, ExecuteQueryArguments{
		OrganizationName: "blocked-secret", WorkspaceName: "ws", QueryConfiguration: "{",
	})
	require.ErrorIs(t, err, terraformclient.ErrOrganizationNotAllowed)
	require.Nil(t, result)
	require.Nil(t, output)
	require.NotContains(t, err.Error(), "blocked-secret")
}

func TestOfficialGeneratorSchemaAndValidation(t *testing.T) {
	schema, err := jsonschema.For[GenerateQueryConfigurationArguments](nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"list_resource_schemas", "provider_name", "provider_namespace", "provider_version"}, schema.Required)
	require.Len(t, schema.Properties, 5)
	require.Equal(t, "string", schema.Properties["resource_types"].Type)
	valid := GenerateQueryConfigurationArguments{
		ListResourceSchemas: `{"example_list":{"block":{"attributes":{}}}}`,
		ProviderName:        "example", ProviderNamespace: "hashicorp", ProviderVersion: "1.2.3",
	}
	for _, field := range schema.Required {
		t.Run(field, func(t *testing.T) {
			input := valid
			switch field {
			case "list_resource_schemas":
				input.ListResourceSchemas = ""
			case "provider_name":
				input.ProviderName = ""
			case "provider_namespace":
				input.ProviderNamespace = ""
			case "provider_version":
				input.ProviderVersion = ""
			}
			result, output, err := GenerateQueryConfigurationFunc(context.Background(), nil, input)
			require.ErrorContains(t, err, "missing required input: "+field)
			require.Nil(t, result)
			require.Nil(t, output)
		})
	}
}

func TestOfficialSearchScopeSchemas(t *testing.T) {
	schema, ok := ProviderListSchemaListTool().InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Empty(t, schema.Required)
	require.Len(t, schema.Properties, 4)
	require.Equal(t, searchlogic.QueryOrganizationDescription, schema.Properties[searchlogic.ScopeOrganizationInput].Description)
	require.Equal(t, searchlogic.QueryWorkspaceDescription, schema.Properties[searchlogic.ScopeWorkspaceInput].Description)
	require.Contains(t, schema.Properties, "provider_namespace")
	require.Contains(t, schema.Properties, "provider_name")
	require.NotContains(t, schema.Properties, "provider_version")
	require.NotContains(t, schema.Properties, "organization_name")
}

func TestOfficialSearchScopeRequestsNativeInput(t *testing.T) {
	for _, test := range []struct {
		name, org, workspace, mcpWorkspace, explicitOrg string
	}{
		{"missing", "", "", "", ""},
		{"complete environment defaults still require confirmation", "org", "ws", "", ""},
		{"mixed explicit and default", "ignored-org", "fallback", "query", "explicit-org"},
		{"blank defaults are unset", " ", "\t", " ", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
			t.Setenv("TF_WORKSPACE", test.workspace)
			t.Setenv("TF_MCP_QUERY_WORKSPACE", test.mcpWorkspace)
			result, output, err := ProviderListSchemaListFunc(context.Background(), nil, ProviderListSchemaListArguments{OrganizationName: test.explicitOrg})
			require.NoError(t, err)
			require.Nil(t, output)
			require.Contains(t, result.InputRequests, searchlogic.ScopeInputRequestID)
			require.Empty(t, result.RequestState)
			require.Empty(t, result.Content)
			execution, executionOutput, err := ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{OrganizationName: test.explicitOrg, QueryConfiguration: "{"})
			require.NoError(t, err)
			require.Nil(t, executionOutput)
			require.Contains(t, execution.InputRequests, searchlogic.ScopeInputRequestID)
			require.Empty(t, execution.RequestState)
			require.Empty(t, execution.Content)
			request := result.InputRequests[searchlogic.ScopeInputRequestID].(*mcp.ElicitParams)
			require.Contains(t, request.Message, searchlogic.ScopeOrganizationInput)
			require.Equal(t, "form", request.Mode)
			schema := officialScopeSchema(searchlogic.ResolveSearchScope(test.explicitOrg, ""))
			require.Equal(t, []string{searchlogic.ScopeOrganizationInput, searchlogic.ScopeWorkspaceInput}, schema.Required)
			require.NotNil(t, schema.AdditionalProperties.Not)
			if test.explicitOrg != "" {
				require.JSONEq(t, `"explicit-org"`, string(schema.Properties[searchlogic.ScopeOrganizationInput].Default))
				require.JSONEq(t, `"query"`, string(schema.Properties[searchlogic.ScopeWorkspaceInput].Default))
			}
		})
	}
}

func TestOfficialScopeAcceptsCompleteScope(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{InputResponses: mcp.InputResponseMap{
		searchlogic.ScopeInputRequestID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{
			searchlogic.ScopeOrganizationInput: " org ",
			searchlogic.ScopeWorkspaceInput:    " workspace ",
		}},
	}}}
	scope, result, err := resolveOfficialScope(context.Background(), request, searchlogic.SearchScope{}, searchlogic.QuerySourceScope)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, "org", scope.Organization.Value)
	require.Equal(t, "workspace", scope.Workspace.Value)
	require.Equal(t, "user_input", scope.Organization.Source)
	require.Equal(t, "user_input", scope.Workspace.Source)
}

func TestOfficialScopeDeclineCancelAndInvalidResponses(t *testing.T) {
	for _, action := range []string{"decline", "cancel", "accept", "invalid"} {
		t.Run(action, func(t *testing.T) {
			request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{InputResponses: mcp.InputResponseMap{
				searchlogic.ScopeInputRequestID: &mcp.ElicitResult{Action: action},
			}}}
			scope, result, err := resolveOfficialScope(context.Background(), request, searchlogic.SearchScope{}, searchlogic.QuerySourceScope)
			require.False(t, scope.Ready())
			if action == "accept" || action == "invalid" {
				require.ErrorContains(t, err, "invalid scope response")
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.Empty(t, result.InputRequests)
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "no scoped API request was made")
			status := "declined"
			if action == "cancel" {
				status = "cancelled"
			}
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, `"status":"`+status+`"`)
		})
	}
}

func TestOfficialScopeRejectsResolvedOrganizationBeforeInputRequest(t *testing.T) {
	ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed-org"})
	proposed := searchlogic.SearchScope{
		Organization: searchlogic.SearchScopeValue{Value: "blocked-org", Source: "TF_CLOUD_ORGANIZATION"},
		Workspace:    searchlogic.SearchScopeValue{Value: "workspace", Source: "TF_WORKSPACE"},
	}

	_, result, err := resolveOfficialScope(ctx, nil, proposed, searchlogic.QuerySourceScope)

	require.ErrorIs(t, err, terraformclient.ErrOrganizationNotAllowed)
	require.Nil(t, result)
	require.NotContains(t, err.Error(), "blocked-org")
}

func TestOfficialScopeRechecksAcceptedOrganization(t *testing.T) {
	ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed-org"})
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{InputResponses: mcp.InputResponseMap{
		searchlogic.ScopeInputRequestID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{
			searchlogic.ScopeOrganizationInput: "blocked-org",
			searchlogic.ScopeWorkspaceInput:    "workspace",
		}},
	}}}
	proposed := searchlogic.SearchScope{
		Organization: searchlogic.SearchScopeValue{Value: "allowed-org", Source: "TF_CLOUD_ORGANIZATION"},
		Workspace:    searchlogic.SearchScopeValue{Value: "workspace", Source: "TF_WORKSPACE"},
	}

	_, result, err := resolveOfficialScope(ctx, request, proposed, searchlogic.QuerySourceScope)

	require.ErrorIs(t, err, terraformclient.ErrOrganizationNotAllowed)
	require.Nil(t, result)
	require.NotContains(t, err.Error(), "blocked-org")
}
