// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchArgumentsAndValidation(t *testing.T) {
	require.Contains(t, GetQueryStatusTool().Description, "query_run_id returned by execute_query")
	require.NotContains(t, GetQueryStatusTool().Description, "latest-query-run")
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema, ok := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, statusSchema.Required, "terminal")
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")
	summarySchema, ok := GetQuerySummaryTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, summarySchema.Properties, "selectable")
	require.Contains(t, summarySchema.Properties, "lists")
	require.NotContains(t, summarySchema.Properties, "has_more")
	require.NotContains(t, summarySchema.Properties, "next_cursor")
	summaryInputSchema, err := jsonschema.For[QuerySummaryArguments](nil)
	require.NoError(t, err)
	require.NotContains(t, summaryInputSchema.Properties, "limit")
	require.NotContains(t, summaryInputSchema.Properties, "after")

	_, _, err = GetQueryStatusFunc(slog.Default())(context.Background(), nil, QueryRunArguments{QueryRunID: " "})
	require.ErrorContains(t, err, "missing required input: query_run_id")
	_, _, err = ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
		OrganizationName: "org", WorkspaceName: "ws", QueryConfiguration: `{`,
	})
	require.ErrorContains(t, err, "invalid query_configuration")
}

func TestOfficialPrepareImportRejectsInvalidProviderBeforeClientUse(t *testing.T) {
	t.Setenv("TF_CLOUD_ORGANIZATION", "org")
	t.Setenv("TF_MCP_IMPORT_WORKSPACE", "target")
	t.Setenv("TF_MCP_QUERY_WORKSPACE", "source-must-not-be-import-target")
	t.Setenv("TF_WORKSPACE", "fallback-must-not-win")
	input := searchlogic.PrepareImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		QueryRunID:       "qry-example",
		Selections:       []searchlogic.ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders:  map[string]string{"aws": "6.18.0"},
	}
	result, output, err := PrepareImportFunc(context.Background(), nil, input)
	require.Nil(t, result)
	require.Nil(t, output)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.ErrorContains(t, err, `locked_providers key "aws"`)
	require.ErrorContains(t, err, "lacks a provider namespace")

	schema, err := jsonschema.For[searchlogic.PrepareImportInput](nil)
	require.NoError(t, err)
	require.Contains(t, schema.Properties["locked_providers"].Description, "Public namespace/type shorthand is accepted")
	require.Contains(t, schema.Properties["locked_providers"].Description, "custom/private sources require hostname/namespace/type")
}

func TestOfficialSearchDescriptionsUseSharedValues(t *testing.T) {
	tests := []struct {
		tool *mcp.Tool
		want string
	}{
		{ProviderListSchemaListTool(), searchlogic.ProviderListSchemaListDescription},
		{GenerateQueryConfigurationTool(), searchlogic.GenerateQueryConfigurationDescription},
		{ExecuteQueryTool(), searchlogic.ExecuteQueryDescription},
		{GetQueryStatusTool(), searchlogic.GetQueryStatusDescription},
		{GetQuerySummaryTool(), searchlogic.GetQuerySummaryDescription},
		{PrepareImportTool(), searchlogic.PrepareImportDescription},
		{VerifyImportPlanTool(), searchlogic.VerifyImportPlanDescription},
	}
	for _, test := range tests {
		require.Equal(t, test.want, test.tool.Description, test.tool.Name)
	}
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
		require.NotContains(t, schema.Required, searchlogic.ScopeOrganizationInput)
		require.NotContains(t, schema.Required, "workspace_name")
		require.Equal(t, searchlogic.QueryOrganizationDescription, schema.Properties[searchlogic.ScopeOrganizationInput].Description)
		require.Equal(t, searchlogic.QueryWorkspaceDescription, schema.Properties["workspace_name"].Description)
		require.NotContains(t, schema.Properties, "organization_name")
	}
	require.Equal(t, []string{"query_configuration"}, querySchema.Required)

	for _, schema := range []*jsonschema.Schema{
		PrepareImportTool().InputSchema.(*jsonschema.Schema),
		VerifyImportPlanTool().InputSchema.(*jsonschema.Schema),
	} {
		require.NotContains(t, schema.Required, searchlogic.ScopeOrganizationInput)
		require.NotContains(t, schema.Required, "workspace_name")
		require.Equal(t, searchlogic.ImportOrganizationDescription, schema.Properties[searchlogic.ScopeOrganizationInput].Description)
		require.Equal(t, searchlogic.ImportWorkspaceDescription, schema.Properties["workspace_name"].Description)
		require.NotContains(t, schema.Properties, "organization_name")
	}
}

func TestOfficialImportToolsRequestMissingTargetScopeBeforeClientUse(t *testing.T) {
	for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE", "TF_WORKSPACE"} {
		t.Setenv(name, "")
	}

	prepareResult, prepareOutput, err := PrepareImportFunc(context.Background(), nil, searchlogic.PrepareImportInput{})
	require.NoError(t, err)
	require.Nil(t, prepareOutput)
	require.Contains(t, prepareResult.InputRequests, searchlogic.ScopeInputRequestID)
	require.Empty(t, prepareResult.Content)

	verifyResult, verifyOutput, err := VerifyImportPlanFunc(context.Background(), nil, searchlogic.VerifyImportInput{})
	require.NoError(t, err)
	require.Nil(t, verifyOutput)
	require.Contains(t, verifyResult.InputRequests, searchlogic.ScopeInputRequestID)
	require.Empty(t, verifyResult.Content)
}

func TestOfficialSearchScopeRequestsNativeInput(t *testing.T) {
	for _, test := range []struct {
		name, org, workspace, mcpWorkspace string
	}{
		{"missing", "", "", ""},
		{"complete environment defaults still require confirmation", "org", "ws", ""},
		{"mixed explicit and default", "org", "", "query"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
			t.Setenv("TF_WORKSPACE", test.workspace)
			t.Setenv("TF_MCP_QUERY_WORKSPACE", test.mcpWorkspace)
			result, _, err := ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{QueryConfiguration: "{"})
			require.NoError(t, err)
			require.Contains(t, result.InputRequests, searchlogic.ScopeInputRequestID)
			require.Empty(t, result.RequestState)
			require.Empty(t, result.Content)
			request := result.InputRequests[searchlogic.ScopeInputRequestID].(*mcp.ElicitParams)
			require.Contains(t, request.Message, searchlogic.ScopeOrganizationInput)
		})
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
