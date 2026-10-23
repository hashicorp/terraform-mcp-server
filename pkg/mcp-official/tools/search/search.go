// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ProviderListSchemaListArguments struct {
	OrganizationName  string `json:"terraform_org_name,omitempty"`
	WorkspaceName     string `json:"workspace_name,omitempty"`
	ProviderNamespace string `json:"provider_namespace,omitempty" jsonschema:"Namespace; supply together with provider_name to fetch a schema"`
	ProviderName      string `json:"provider_name,omitempty" jsonschema:"Short name; supply together with provider_namespace to fetch a schema"`
}

func tool(name, description, title string, readOnly, openWorld bool) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title: title, OpenWorldHint: jsonschema.Ptr(openWorld),
			ReadOnlyHint: readOnly, DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// searchScopeInputSchema applies shared descriptions that cannot be referenced in struct tags.
func searchScopeInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties[searchlogic.ScopeOrganizationInput].Description = searchlogic.QueryOrganizationDescription
	schema.Properties["workspace_name"].Description = searchlogic.QueryWorkspaceDescription
	return schema
}

func ProviderListSchemaListTool() *mcp.Tool {
	providerTool := tool("provider_list_schema_list", searchlogic.ProviderListSchemaListDescription,
		"Fetch list_resource_schemas for a search-compatible provider", true, true)
	providerTool.InputSchema = searchScopeInputSchema[ProviderListSchemaListArguments]()
	return providerTool
}

func ProviderListSchemaListFunc(ctx context.Context, req *mcp.CallToolRequest, input ProviderListSchemaListArguments) (*mcp.CallToolResult, any, error) {
	scope := searchlogic.ResolveSearchScope(input.OrganizationName, input.WorkspaceName)
	scope, interactionResult, err := resolveOfficialScope(ctx, req, scope, searchlogic.QuerySourceScope)
	if err != nil || interactionResult != nil {
		return interactionResult, nil, err
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, searchlogic.TerraformClientUnavailableError(err)
	}
	text, err := searchlogic.ProviderListSchemaListText(ctx, tfeClient, scope.Organization.Value, scope.Workspace.Value, input.ProviderNamespace, input.ProviderName)
	if err != nil {
		return nil, nil, searchlogic.ProviderDataUnavailableError(err)
	}
	return textResult(text), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
