// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

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

func TestOfficialImportScopeSchemas(t *testing.T) {
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

func TestOfficialVerifySchemaAndTargetInteraction(t *testing.T) {
	schema := VerifyImportPlanTool().InputSchema.(*jsonschema.Schema)
	require.Equal(t, []string{"run_id"}, schema.Required)
	for _, forbidden := range []string{"plan_id", "configuration_version_id", "query_run_id"} {
		require.NotContains(t, schema.Properties, forbidden)
	}
	for _, field := range []string{"import_context", "bindings", "detail_addresses"} {
		require.Contains(t, schema.Properties, field)
	}
	output := VerifyImportPlanTool().OutputSchema.(*jsonschema.Schema)
	for _, field := range []string{"status", "run_id", "plan_id", "changes", "object_identity", "diagnostics", "next_action"} {
		require.Contains(t, output.Properties, field)
	}
	t.Setenv("TF_CLOUD_ORGANIZATION", "org")
	t.Setenv("TF_MCP_IMPORT_WORKSPACE", "target")
	t.Setenv("TF_MCP_QUERY_WORKSPACE", "source-must-not-be-import-target")
	t.Setenv("TF_WORKSPACE", "fallback")
	result, verified, err := VerifyImportPlanFunc(context.Background(), nil, searchlogic.VerifyImportInput{})
	require.NoError(t, err)
	require.Nil(t, verified)
	request := result.InputRequests[searchlogic.ScopeInputRequestID].(*mcp.ElicitParams)
	require.Contains(t, request.Message, "import target")
	require.Contains(t, request.Message, `workspace_name="target"`)
	require.NotContains(t, request.Message, "source-must-not-be-import-target")
	for _, action := range []string{"decline", "cancel"} {
		req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{InputResponses: mcp.InputResponseMap{
			searchlogic.ScopeInputRequestID: &mcp.ElicitResult{Action: action},
		}}}
		result, verified, err := VerifyImportPlanFunc(context.Background(), req, searchlogic.VerifyImportInput{})
		require.NoError(t, err)
		require.Nil(t, verified)
		require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "no scoped API request")
	}
}
