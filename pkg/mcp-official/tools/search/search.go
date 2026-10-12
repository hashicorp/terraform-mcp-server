// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	QueryRunID string `json:"query_run_id" jsonschema:"Exact query_run_id returned by execute_query or an existing authorized QueryRun ID"`
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
	return tool("generate_query_configuration", searchlogic.GenerateQueryConfigurationDescription,
		"Generate a No-Code Query Configuration from a list_resource_schemas schema", true, false)
}

func GenerateQueryConfigurationFunc(_ context.Context, _ *mcp.CallToolRequest, input GenerateQueryConfigurationArguments) (*mcp.CallToolResult, any, error) {
	for _, field := range []struct{ name, value string }{
		{"list_resource_schemas", input.ListResourceSchemas}, {"provider_name", input.ProviderName},
		{"provider_namespace", input.ProviderNamespace}, {"provider_version", input.ProviderVersion},
	} {
		if field.value == "" {
			return nil, nil, fmt.Errorf("missing required input: %s", field.name)
		}
	}
	text, err := searchlogic.GenerateQueryConfigurationText(input.ListResourceSchemas, input.ProviderName, input.ProviderNamespace, input.ProviderVersion, input.ResourceTypes)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text), nil, nil
}

func ProviderListSchemaListTool() *mcp.Tool {
	return tool("provider_list_schema_list", searchlogic.ProviderListSchemaListDescription,
		"Fetch list_resource_schemas for a search-compatible provider", true, true)
}

func ProviderListSchemaListFunc(ctx context.Context, req *mcp.CallToolRequest, input ProviderListSchemaListArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.OrganizationName) == "" || strings.TrimSpace(input.WorkspaceName) == "" {
		return nil, nil, fmt.Errorf("organization_name and workspace_name are required; ask the user to provide both before fetching provider schemas")
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}
	text, err := searchlogic.ProviderListSchemaListText(ctx, tfeClient, input.OrganizationName, input.WorkspaceName, input.ProviderNamespace, input.ProviderName)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text), nil, nil
}

func ExecuteQueryTool() *mcp.Tool {
	queryTool := tool("execute_query", searchlogic.ExecuteQueryDescription,
		"Create and execute an HCP Terraform no-code query", false, true)
	schema, err := jsonschema.For[searchlogic.ExecuteQueryResponse](nil)
	if err != nil {
		panic(err)
	}
	queryTool.OutputSchema = schema
	return queryTool
}

func ExecuteQueryFunc(ctx context.Context, req *mcp.CallToolRequest, input ExecuteQueryArguments) (*mcp.CallToolResult, *searchlogic.ExecuteQueryResponse, error) {
	if strings.TrimSpace(input.OrganizationName) == "" || strings.TrimSpace(input.WorkspaceName) == "" || strings.TrimSpace(input.QueryConfiguration) == "" {
		return nil, nil, fmt.Errorf("organization_name, workspace_name and query_configuration are required")
	}
	if err := searchlogic.ValidateQueryConfiguration(input.QueryConfiguration); err != nil {
		return nil, nil, fmt.Errorf("invalid query_configuration: %w", err)
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client; check session configuration")
	}
	response, err := searchlogic.SubmitQuery(ctx, tfeClient, input.OrganizationName, input.WorkspaceName, input.QueryConfiguration)
	if err != nil {
		return nil, nil, err
	}
	return textResult(fmt.Sprintf(`{"query_run_id":%q}`, response.QueryRunID)), response, nil
}

func GetQueryStatusTool() *mcp.Tool {
	statusTool := tool("get_query_status", searchlogic.GetQueryStatusDescription,
		"Get HCP Terraform query run status", true, true)
	schema, err := jsonschema.For[searchlogic.QueryStatusResponse](nil)
	if err != nil {
		panic(err)
	}
	statusTool.OutputSchema = schema
	return statusTool
}

func GetQueryStatusFunc(logger *slog.Logger) mcp.ToolHandlerFor[QueryRunArguments, *searchlogic.QueryStatusResponse] {
	searchLogger := logging.WrapSlog(logger)
	return func(ctx context.Context, req *mcp.CallToolRequest, input QueryRunArguments) (*mcp.CallToolResult, *searchlogic.QueryStatusResponse, error) {
		queryRunID := strings.TrimSpace(input.QueryRunID)
		if queryRunID == "" {
			return nil, nil, fmt.Errorf("missing required input: query_run_id")
		}
		tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
		}
		response, err := searchlogic.PollQueryStatus(ctx, tfeClient, queryRunID, searchLogger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get query run %q: %w", queryRunID, err)
		}
		text, err := searchlogic.FormatQueryStatus(response)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal query run %q: %w", queryRunID, err)
		}
		return textResult(text), response, nil
	}
}

func GetQuerySummaryTool() *mcp.Tool {
	return tool("get_query_summary", searchlogic.GetQuerySummaryDescription,
		"Get HCP Terraform query summary", true, true)
}

func GetQuerySummaryFunc(ctx context.Context, req *mcp.CallToolRequest, input QueryRunArguments) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.QueryRunID) == "" {
		return nil, nil, fmt.Errorf("missing required input: query_run_id")
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}
	text, err := searchlogic.ReadQuerySummary(ctx, tfeClient, input.QueryRunID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get query summary for %q: %w", input.QueryRunID, err)
	}
	return textResult(text), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
