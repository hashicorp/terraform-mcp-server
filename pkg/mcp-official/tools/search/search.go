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
	OrganizationName  string `json:"organization_name,omitempty"`
	WorkspaceName     string `json:"workspace_name,omitempty"`
	ProviderNamespace string `json:"provider_namespace,omitempty" jsonschema:"Namespace; supply together with provider_name to fetch a schema"`
	ProviderName      string `json:"provider_name,omitempty" jsonschema:"Short name; supply together with provider_namespace to fetch a schema"`
}

type ExecuteQueryArguments struct {
	OrganizationName   string `json:"organization_name,omitempty"`
	WorkspaceName      string `json:"workspace_name,omitempty"`
	QueryConfiguration string `json:"query_configuration" jsonschema:"JSON no-code query configuration produced using provider_list_schema_list and generate_query_configuration"`
}

type QueryRunArguments struct {
	QueryRunID string `json:"query_run_id" jsonschema:"Exact query_run_id returned by execute_query or an existing authorized QueryRun ID"`
}

type QuerySummaryArguments struct {
	QueryRunID   string `json:"query_run_id" jsonschema:"Exact QueryRun ID returned by execute_query or an existing authorized QueryRun ID"`
	ResourceType string `json:"resource_type,omitempty" jsonschema:"Optional exact Search list-resource type filter"`
	Address      string `json:"address,omitempty" jsonschema:"Optional exact list block address filter"`
	NameContains string `json:"name_contains,omitempty" jsonschema:"Optional case-insensitive display-name substring"`
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

// searchScopeInputSchema applies shared descriptions that cannot be referenced in struct tags.
func searchScopeInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["organization_name"].Description = searchlogic.QueryOrganizationDescription
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
	if !scope.Ready() {
		return textResult(scope.InputRequest()), nil, nil
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}
	text, err := searchlogic.ProviderListSchemaListText(ctx, tfeClient, scope.Organization.Value, scope.Workspace.Value, input.ProviderNamespace, input.ProviderName)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text), nil, nil
}

func ExecuteQueryTool() *mcp.Tool {
	queryTool := tool("execute_query", searchlogic.ExecuteQueryDescription,
		"Create and execute an HCP Terraform no-code query", false, true)
	queryTool.InputSchema = searchScopeInputSchema[ExecuteQueryArguments]()
	schema, err := jsonschema.For[searchlogic.ExecuteQueryResponse](nil)
	if err != nil {
		panic(err)
	}
	queryTool.OutputSchema = schema
	return queryTool
}

func ExecuteQueryFunc(ctx context.Context, req *mcp.CallToolRequest, input ExecuteQueryArguments) (*mcp.CallToolResult, *searchlogic.ExecuteQueryResponse, error) {
	scope := searchlogic.ResolveSearchScope(input.OrganizationName, input.WorkspaceName)
	if !scope.Ready() {
		return textResult(scope.InputRequest()), nil, nil
	}
	if strings.TrimSpace(input.QueryConfiguration) == "" {
		return nil, nil, fmt.Errorf("missing required input: query_configuration")
	}
	if err := searchlogic.ValidateQueryConfiguration(input.QueryConfiguration); err != nil {
		return nil, nil, fmt.Errorf("invalid query_configuration: %w", err)
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client; check session configuration")
	}
	response, err := searchlogic.SubmitQuery(ctx, tfeClient, scope.Organization.Value, scope.Workspace.Value, input.QueryConfiguration)
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
	summaryTool := tool("get_query_summary", searchlogic.GetQuerySummaryDescription,
		"Get HCP Terraform query summary", true, true)
	schema, err := jsonschema.For[searchlogic.QueryDiscoverySummary](nil)
	if err != nil {
		panic(err)
	}
	summaryTool.OutputSchema = schema
	return summaryTool
}

func GetQuerySummaryFunc(ctx context.Context, req *mcp.CallToolRequest, input QuerySummaryArguments) (*mcp.CallToolResult, *searchlogic.QueryDiscoverySummary, error) {
	queryRunID := strings.TrimSpace(input.QueryRunID)
	if queryRunID == "" {
		return nil, nil, fmt.Errorf("missing required input: query_run_id")
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client; check session configuration")
	}
	filter := searchlogic.QueryDiscoveryFilter{ResourceType: input.ResourceType, Address: input.Address, NameContains: input.NameContains}
	summary, err := searchlogic.ReadQueryDiscoverySummary(ctx, tfeClient, queryRunID, filter)
	if err != nil {
		return nil, nil, fmt.Errorf("query summary unavailable: %w", err)
	}
	text, err := searchlogic.FormatQueryDiscoverySummary(summary)
	if err != nil {
		return nil, nil, fmt.Errorf("query summary unavailable: %w", err)
	}
	return textResult(text), summary, nil
}

func PrepareImportTool() *mcp.Tool {
	t := tool("prepare_import", searchlogic.PrepareImportDescription, "Prepare explicit Search import selection", true, true)
	inputSchema, err := jsonschema.For[searchlogic.PrepareImportInput](nil)
	if err != nil {
		panic(err)
	}
	inputSchema.Properties["organization_name"].Description = searchlogic.ImportOrganizationDescription
	inputSchema.Properties["workspace_name"].Description = searchlogic.ImportWorkspaceDescription
	t.InputSchema = inputSchema
	schema, err := jsonschema.For[searchlogic.PrepareImportResult](nil)
	if err != nil {
		panic(err)
	}
	t.OutputSchema = schema
	return t
}

func PrepareImportFunc(ctx context.Context, req *mcp.CallToolRequest, input searchlogic.PrepareImportInput) (*mcp.CallToolResult, *searchlogic.PrepareImportResult, error) {
	scope := searchlogic.ResolveImportScope(input.OrganizationName, input.WorkspaceName)
	if !scope.Ready() {
		return textResult(scope.ImportInputRequest()), nil, nil
	}
	input.OrganizationName, input.WorkspaceName = scope.Organization.Value, scope.Workspace.Value
	if err := searchlogic.ValidatePrepareImportInput(input); err != nil {
		return nil, nil, fmt.Errorf("validating prepare_import arguments: %w", err)
	}
	c, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, searchlogic.TerraformClientUnavailableError(err)
	}
	result, text, err := searchlogic.PrepareImportToolOutput(ctx, c, input)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text), result, nil
}

func VerifyImportPlanTool() *mcp.Tool {
	t := tool("verify_import_plan", searchlogic.VerifyImportPlanDescription, "Inspect exact CLI speculative Run and Plan", true, true)
	inputSchema, err := jsonschema.For[searchlogic.VerifyImportInput](nil)
	if err != nil {
		panic(err)
	}
	inputSchema.Properties["organization_name"].Description = searchlogic.ImportOrganizationDescription
	inputSchema.Properties["workspace_name"].Description = searchlogic.ImportWorkspaceDescription
	t.InputSchema = inputSchema
	schema, err := jsonschema.For[searchlogic.VerifyImportResult](nil)
	if err != nil {
		panic(err)
	}
	t.OutputSchema = schema
	return t
}

func VerifyImportPlanFunc(ctx context.Context, req *mcp.CallToolRequest, input searchlogic.VerifyImportInput) (*mcp.CallToolResult, *searchlogic.VerifyImportResult, error) {
	scope := searchlogic.ResolveImportScope(input.OrganizationName, input.WorkspaceName)
	if !scope.Ready() {
		return textResult(scope.ImportInputRequest()), nil, nil
	}
	input.OrganizationName, input.WorkspaceName = scope.Organization.Value, scope.Workspace.Value
	c, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(req))
	if err != nil {
		return nil, nil, searchlogic.TerraformClientUnavailableError(err)
	}
	result, text, err := searchlogic.VerifyImportPlanToolOutput(ctx, c, input)
	if err != nil {
		return nil, nil, err
	}
	return textResult(text), result, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
