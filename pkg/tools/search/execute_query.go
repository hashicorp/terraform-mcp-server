// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

type executeQueryConfiguration struct {
	GenerateConfigOut *bool                 `json:"generate_config_out,omitempty"`
	Providers         []noCodeQueryProvider `json:"no_code_query_providers"`
}

type noCodeQueryProvider struct {
	Namespace string                `json:"namespace"`
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	Resources []noCodeQueryResource `json:"no_code_query_resources"`
}

type noCodeQueryResource struct {
	Body map[string]any `json:"body"`
}

type executeQueryOptions struct {
	Type              string                `jsonapi:"primary,no-code-queries"`
	Source            string                `jsonapi:"attr,source"`
	GenerateConfigOut *bool                 `jsonapi:"attr,generate-config-out,omitempty"`
	Providers         []noCodeQueryProvider `jsonapi:"attr,no-code-query-providers"`
	Workspace         *tfe.Workspace        `jsonapi:"relation,workspace"`
}

const executeQuerySource = "terraform-mcp"

// ExecuteQuery creates and immediately executes an HCP Terraform no-code query.
func ExecuteQuery(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("execute_query",
			mcp.WithDescription(executeQueryDescription),
			mcp.WithTitleAnnotation("Create and execute an HCP Terraform no-code query"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("organization_name",
				mcp.Required(),
				mcp.Description("Name of the HCP Terraform organization containing the workspace."),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("Name of the HCP Terraform workspace in which to create and execute the query. The tool resolves its ID using go-tfe."),
			),
			mcp.WithString("query_configuration",
				mcp.Required(),
				mcp.Description(
					"JSON object containing no_code_query_providers and optional generate_config_out. "+
						"Use provider_list_schema_list and generate_query_configuration to construct this value. "+
						"Every resource_type must be an exact list_resource_schemas key; ordinary managed "+
						"resource names are not necessarily supported list resources. The tool adds the JSON:API "+
						"envelope and workspace relationship automatically.",
				),
			),
			mcp.WithOutputSchema[ExecuteQueryResponse](),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return executeQueryHandler(ctx, request, logger)
		},
	}
}

func executeQueryHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	organizationName, err := request.RequireString("organization_name")
	if err != nil || strings.TrimSpace(organizationName) == "" {
		return toolErrorf(logger, "execute_query", "missing required input: organization_name")
	}
	organizationName = strings.TrimSpace(organizationName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil || strings.TrimSpace(workspaceName) == "" {
		return toolErrorf(logger, "execute_query", "missing required input: workspace_name")
	}
	workspaceName = strings.TrimSpace(workspaceName)

	rawConfiguration, err := request.RequireString("query_configuration")
	if err != nil || strings.TrimSpace(rawConfiguration) == "" {
		return toolErrorf(logger, "execute_query", "missing required input: query_configuration")
	}

	err = ValidateQueryConfiguration(rawConfiguration)
	if err != nil {
		return toolErrorf(logger, "execute_query", "invalid query_configuration: %v", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return toolErrorf(logger, "execute_query", "failed to get Terraform client; check session configuration")
	}
	return executeQueryWithClient(ctx, tfeClient, organizationName, workspaceName, rawConfiguration, logger)
}

func executeQueryWithClient(ctx context.Context, tfeClient *tfe.Client, organizationName, workspaceName, rawConfiguration string, logger *log.Logger) (*mcp.CallToolResult, error) {
	result, err := SubmitQuery(ctx, tfeClient, organizationName, workspaceName, rawConfiguration)
	if err != nil {
		return toolErrorf(logger, "execute_query", "%v", err)
	}
	text, err := json.Marshal(result)
	if err != nil {
		return toolErrorf(logger, "execute_query", "failed to format query result")
	}
	return mcp.NewToolResultStructured(result, string(text)), nil
}

// ExecuteQueryResponse contains the sole backend field exposed after query creation.
type ExecuteQueryResponse struct {
	QueryRunID string `json:"query_run_id"`
}

// ValidateQueryConfiguration rejects malformed no-code query inputs before any API call.
func ValidateQueryConfiguration(rawConfiguration string) error {
	_, err := parseExecuteQueryConfiguration(rawConfiguration)
	return err
}

// ExecuteQueryText validates the input and submits one no-code query to an existing workspace.
// The caller supplies its transport's authenticated go-tfe client.
func ExecuteQueryText(ctx context.Context, tfeClient *tfe.Client, organizationName, workspaceName, rawConfiguration string) (string, error) {
	result, err := SubmitQuery(ctx, tfeClient, organizationName, workspaceName, rawConfiguration)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to format query result")
	}
	return string(encoded), nil
}

// SubmitQuery makes one create request. A failed POST or untrustworthy response
// has an unknown outcome: the caller must reconcile before submitting again.
func SubmitQuery(ctx context.Context, tfeClient *tfe.Client, organizationName, workspaceName, rawConfiguration string) (*ExecuteQueryResponse, error) {
	if organizationName = strings.TrimSpace(organizationName); organizationName == "" {
		return nil, fmt.Errorf("missing required input: organization_name")
	}
	if workspaceName = strings.TrimSpace(workspaceName); workspaceName == "" {
		return nil, fmt.Errorf("missing required input: workspace_name")
	}
	if strings.TrimSpace(rawConfiguration) == "" {
		return nil, fmt.Errorf("missing required input: query_configuration")
	}
	configuration, err := parseExecuteQueryConfiguration(rawConfiguration)
	if err != nil {
		return nil, fmt.Errorf("invalid query_configuration: %w", err)
	}
	workspace, err := tfeClient.Workspaces.Read(ctx, organizationName, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("unable to read source workspace; check access and workspace selection")
	}

	response, err := submitExecuteQuery(ctx, tfeClient, workspace.ID, configuration)
	if err != nil {
		return nil, fmt.Errorf("query creation outcome unknown; reconcile before submitting another query")
	}

	return response, nil
}

func parseExecuteQueryConfiguration(raw string) (*executeQueryConfiguration, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var configuration executeQueryConfiguration
	if err := decoder.Decode(&configuration); err != nil {
		return nil, fmt.Errorf("input is not valid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("input contains more than one JSON value")
	}

	providers := configuration.Providers
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one no_code_query_providers entry is required")
	}
	for providerIndex, provider := range providers {
		if strings.TrimSpace(provider.Namespace) == "" {
			return nil, fmt.Errorf("provider %d namespace is required", providerIndex)
		}
		if strings.TrimSpace(provider.Name) == "" {
			return nil, fmt.Errorf("provider %d name is required", providerIndex)
		}
		if strings.TrimSpace(provider.Version) == "" {
			return nil, fmt.Errorf("provider %d version is required", providerIndex)
		}
		if len(provider.Resources) == 0 {
			return nil, fmt.Errorf("provider %d must contain at least one no_code_query_resources entry", providerIndex)
		}

		for resourceIndex, resource := range provider.Resources {
			resourceType, ok := resource.Body["resource_type"].(string)
			if !ok || strings.TrimSpace(resourceType) == "" {
				return nil, fmt.Errorf("provider %d resource %d body.resource_type is required", providerIndex, resourceIndex)
			}
			if limit, ok := resource.Body["limit"]; ok && !isPositiveInteger(limit) {
				return nil, fmt.Errorf("provider %d resource %d body.limit must be a positive integer", providerIndex, resourceIndex)
			}
			if attributes, ok := resource.Body["attributes"]; ok {
				items, ok := attributes.([]any)
				if !ok {
					return nil, fmt.Errorf("provider %d resource %d body.attributes must be an array", providerIndex, resourceIndex)
				}
				for attributeIndex, attribute := range items {
					entry, ok := attribute.(map[string]any)
					name, hasName := entry["attribute"].(string)
					if !ok || !hasName || strings.TrimSpace(name) == "" {
						return nil, fmt.Errorf("provider %d resource %d attribute %d name is required", providerIndex, resourceIndex, attributeIndex)
					}
				}
			}
		}
	}

	return &configuration, nil
}

func isPositiveInteger(value any) bool {
	switch v := value.(type) {
	case json.Number:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		return err == nil && n > 0
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return err == nil && n > 0
	default:
		return false
	}
}

// The create response should be tiny; reject excessive data rather than buffering
// an arbitrary Atlas response. This is not a bound on query results.
const maxQueryCreateResponseBytes = 64 << 10

type boundedQueryCreateResponse struct{ buffer bytes.Buffer }

func (b *boundedQueryCreateResponse) Write(p []byte) (int, error) {
	if len(p) > maxQueryCreateResponseBytes-b.buffer.Len() {
		return 0, fmt.Errorf("query create response exceeds limit")
	}
	return b.buffer.Write(p)
}

func validQueryRunID(id string) bool {
	if !strings.HasPrefix(id, "qry-") || len(id) <= len("qry-") || len(id) > 128 {
		return false
	}
	for _, c := range id[len("qry-"):] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func submitExecuteQuery(ctx context.Context, tfeClient *tfe.Client, workspaceID string, configuration *executeQueryConfiguration) (*ExecuteQueryResponse, error) {
	options := &executeQueryOptions{
		Source:            executeQuerySource,
		GenerateConfigOut: configuration.GenerateConfigOut,
		Providers:         configuration.Providers,
		Workspace: &tfe.Workspace{
			ID: workspaceID,
		},
	}

	request, err := tfeClient.NewRequest(http.MethodPost, "search/no-code-query", options)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	var response boundedQueryCreateResponse
	if err := request.Do(ctx, &response); err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Relationships struct {
				LatestQueryRun struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"latest-query-run"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.buffer.Bytes(), &payload); err != nil || !validQueryRunID(payload.Data.Relationships.LatestQueryRun.Data.ID) {
		return nil, fmt.Errorf("query create response did not contain a valid query run ID")
	}
	return &ExecuteQueryResponse{QueryRunID: payload.Data.Relationships.LatestQueryRun.Data.ID}, nil
}

const executeQueryDescription = `Creates and immediately executes an HCP Terraform Search query.

MANDATORY WORKFLOW: Before calling this tool, call provider_list_schema_list and then
generate_query_configuration. Do not skip generate_query_configuration or construct the payload
directly. Pass an organization name, workspace name, and the provider/resource configuration
produced from its guidance.
Never construct resource_type from an ordinary managed resource name: it must be an exact key
in list_resource_schemas for the selected provider version. If no matching key exists, do not
call this tool. The tool resolves the workspace
with go-tfe, constructs the JSON:API request, and
calls POST /api/v2/search/no-code-query, which persists the selections, uploads generated
Terraform query configuration, and starts a query run. It does not create or modify
infrastructure.

The response includes only query_run_id. Pass that query run ID to get_query_status.
After it returns a terminal status, pass the same ID to
get_query_summary to retrieve the parsed result. Do not use curl or call the HCP
Terraform query API directly.

Requires an authenticated HCP Terraform session and a workspace using a supported
Terraform version.`
