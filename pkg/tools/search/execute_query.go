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
					if _, hasValue := entry["value"]; !hasValue {
						return nil, fmt.Errorf("provider %d resource %d attribute %d value is required", providerIndex, resourceIndex, attributeIndex)
					}
				}
			}
		}
	}

	return &configuration, nil
}

func isPositiveInteger(value any) bool {
	v, ok := value.(json.Number)
	if !ok {
		return false
	}
	n, err := strconv.ParseInt(v.String(), 10, 64)
	return err == nil && n > 0
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

	var response bytes.Buffer
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
	if err := json.Unmarshal(response.Bytes(), &payload); err != nil || strings.TrimSpace(payload.Data.Relationships.LatestQueryRun.Data.ID) == "" {
		return nil, fmt.Errorf("query create response did not contain a query run ID")
	}
	return &ExecuteQueryResponse{QueryRunID: payload.Data.Relationships.LatestQueryRun.Data.ID}, nil
}

const executeQueryDescription = `Creates and immediately executes one HCP Terraform No-Code Search query. It
creates Search query records and a QueryRun, but does not create or modify cloud
infrastructure.

Before calling, use provider_list_schema_list and
generate_query_configuration for the same source organization/workspace. Pass
the schema-backed provider/resource configuration produced from that guidance;
resource_type must be an exact list_resource_schemas key. Do not construct the
payload directly from an ordinary managed-resource name.
Do not add a requested condition that is absent from the list-resource schema;
follow the generator guidance for generate_config_out when later selection
depends on additional result data.

Established success returns only query_run_id. Pass that exact ID to
get_query_status. If creation does not return a trustworthy ID, reconcile the
outcome before another submission; do not blindly execute the query again.`
