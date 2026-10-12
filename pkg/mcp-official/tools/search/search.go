// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	legacy "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	oldmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GenerateQueryConfigurationArguments struct {
	ListResourceSchemas string `json:"list_resource_schemas" jsonschema:"JSON list_resource_schemas object returned by provider_list_schema_list"`
	ProviderName        string `json:"provider_name" jsonschema:"Short provider name, e.g. aws"`
	ProviderNamespace   string `json:"provider_namespace" jsonschema:"Provider namespace, e.g. hashicorp"`
	ProviderVersion     string `json:"provider_version" jsonschema:"Exact catalog provider version"`
	ResourceTypes       string `json:"resource_types,omitempty" jsonschema:"Optional comma-separated exact list resource type keys"`
}

type ProviderListSchemaListArguments struct {
	OrganizationName  string `json:"organization_name" jsonschema:"HCP Terraform organization used to scope catalog requests"`
	WorkspaceName     string `json:"workspace_name" jsonschema:"Existing HCP Terraform workspace that will execute the query"`
	ProviderNamespace string `json:"provider_namespace,omitempty" jsonschema:"Namespace; supply together with provider_name to fetch a schema"`
	ProviderName      string `json:"provider_name,omitempty" jsonschema:"Short name; supply together with provider_namespace to fetch a schema"`
}

type ExecuteQueryArguments struct {
	OrganizationName   string `json:"organization_name" jsonschema:"HCP Terraform organization containing the workspace"`
	WorkspaceName      string `json:"workspace_name" jsonschema:"HCP Terraform workspace in which to execute the query"`
	QueryConfiguration string `json:"query_configuration" jsonschema:"JSON no-code query configuration produced using provider_list_schema_list and generate_query_configuration"`
}

type QueryRunArguments struct {
	QueryRunID string `json:"query_run_id" jsonschema:"Exact query run ID returned by execute_query"`
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

func GenerateQueryConfigurationTool() *mcp.Tool {
	return tool("generate_query_configuration", legacy.GenerateQueryConfiguration(log.StandardLogger()).Tool.Description,
		"Generate a No-Code Query Configuration from a list_resource_schemas schema", true, false)
}

func GenerateQueryConfigurationFunc(_ context.Context, _ *mcp.CallToolRequest, input GenerateQueryConfigurationArguments) (*mcp.CallToolResult, any, error) {
	for _, field := range []struct{ name, value string }{
		{"list_resource_schemas", input.ListResourceSchemas}, {"provider_name", input.ProviderName},
		{"provider_namespace", input.ProviderNamespace}, {"provider_version", input.ProviderVersion},
	} {
		if field.value == "" {
			return toolError(fmt.Sprintf("missing required input: %s", field.name)), nil, nil
		}
	}
	text, err := legacy.GenerateQueryConfigurationText(input.ListResourceSchemas, input.ProviderName, input.ProviderNamespace, input.ProviderVersion, input.ResourceTypes)
	if err != nil {
		return toolError(err.Error()), nil, nil
	}
	return textResult(text), nil, nil
}

func ProviderListSchemaListTool() *mcp.Tool {
	return tool("provider_list_schema_list", legacy.ProviderListSchemaList(log.StandardLogger()).Tool.Description,
		"Fetch list_resource_schemas for a search-compatible provider", true, true)
}

func ProviderListSchemaListFunc(ctx context.Context, req *mcp.CallToolRequest, input ProviderListSchemaListArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.OrganizationName) == "" || strings.TrimSpace(input.WorkspaceName) == "" {
		return toolError("organization_name and workspace_name are required; ask the user to provide both before fetching provider schemas"), nil, nil
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return toolError(fmt.Sprintf("failed to get Terraform client: %v", err)), nil, nil
	}
	result, err := legacy.ProviderListSchemaListWithClient(ctx, tfeClient, input.OrganizationName, input.WorkspaceName, input.ProviderNamespace, input.ProviderName, log.StandardLogger())
	return convertResult(result, err)
}

func ExecuteQueryTool() *mcp.Tool {
	return tool("execute_query", legacy.ExecuteQuery(log.StandardLogger()).Tool.Description,
		"Create and execute an HCP Terraform no-code query", false, true)
}

func ExecuteQueryFunc(ctx context.Context, req *mcp.CallToolRequest, input ExecuteQueryArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.OrganizationName) == "" || strings.TrimSpace(input.WorkspaceName) == "" || strings.TrimSpace(input.QueryConfiguration) == "" {
		return toolError("organization_name, workspace_name and query_configuration are required"), nil, nil
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return toolError(fmt.Sprintf("failed to get Terraform client: %v", err)), nil, nil
	}
	result, err := legacy.ExecuteQueryWithClient(ctx, tfeClient, input.OrganizationName, input.WorkspaceName, input.QueryConfiguration, log.StandardLogger())
	return convertResult(result, err)
}

func GetQueryStatusTool() *mcp.Tool {
	statusTool := tool("get_query_status", legacy.GetQueryStatus(log.StandardLogger()).Tool.Description,
		"Get HCP Terraform query run status", true, true)
	statusTool.OutputSchema = &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"query_run_id":        {Type: "string"},
			"status":              {Type: "string"},
			"terminal":            {Type: "boolean"},
			"retry_after_seconds": {Type: "integer"},
			"message":             {Type: "string"},
			"status_timestamps":   {Type: "object"},
		},
		Required: []string{"query_run_id", "status", "terminal", "message"},
	}
	return statusTool
}

func GetQueryStatusFunc(ctx context.Context, req *mcp.CallToolRequest, input QueryRunArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.QueryRunID) == "" {
		return toolError("missing required input: query_run_id"), nil, nil
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return toolError(fmt.Sprintf("failed to get Terraform client: %v", err)), nil, nil
	}
	result, err := legacy.GetQueryStatusWithClient(ctx, tfeClient, input.QueryRunID, log.StandardLogger())
	return convertResult(result, err)
}

func GetQuerySummaryTool() *mcp.Tool {
	return tool("get_query_summary", legacy.GetQuerySummary(log.StandardLogger()).Tool.Description,
		"Get HCP Terraform query summary", true, true)
}

func GetQuerySummaryFunc(ctx context.Context, req *mcp.CallToolRequest, input QueryRunArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.QueryRunID) == "" {
		return toolError("missing required input: query_run_id"), nil, nil
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return toolError(fmt.Sprintf("failed to get Terraform client: %v", err)), nil, nil
	}
	result, err := legacy.GetQuerySummaryWithClient(ctx, tfeClient, input.QueryRunID, log.StandardLogger())
	return convertResult(result, err)
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func toolError(message string) *mcp.CallToolResult {
	result := textResult(message)
	result.IsError = true
	return result
}

// convertResult copies the existing Search text/structured response onto the official wire.
// Do not synthesize a new client session or forward credentials through legacy SDK context.
func convertResult(result *oldmcp.CallToolResult, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return toolError(err.Error()), nil, nil
	}
	if result == nil {
		return toolError("Search returned no result"), nil, nil
	}
	converted := &mcp.CallToolResult{IsError: result.IsError, StructuredContent: result.StructuredContent}
	for _, content := range result.Content {
		switch value := content.(type) {
		case oldmcp.TextContent:
			converted.Content = append(converted.Content, &mcp.TextContent{Text: value.Text})
		default:
			return toolError("Search returned unsupported content"), nil, nil
		}
	}
	return converted, nil, nil
}
