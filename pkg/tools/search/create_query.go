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

type createQueryConfiguration struct {
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

type createQueryOptions struct {
	Type              string                `jsonapi:"primary,no-code-queries"`
	GenerateConfigOut *bool                 `jsonapi:"attr,generate-config-out,omitempty"`
	Providers         []noCodeQueryProvider `jsonapi:"attr,no-code-query-providers"`
	Workspace         *tfe.Workspace        `jsonapi:"relation,workspace"`
}

// CreateQuery creates and immediately executes an HCP Terraform no-code query.
func CreateQuery(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_query",
			mcp.WithDescription(createQueryDescription),
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
						"Use generate_query_configuration to construct this value. The tool adds the JSON:API "+
						"envelope and workspace relationship automatically.",
				),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createQueryHandler(ctx, request, logger)
		},
	}
}

func createQueryHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	organizationName, err := request.RequireString("organization_name")
	if err != nil || strings.TrimSpace(organizationName) == "" {
		return createQueryToolErrorf(logger, "missing required input: organization_name")
	}
	organizationName = strings.TrimSpace(organizationName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil || strings.TrimSpace(workspaceName) == "" {
		return createQueryToolErrorf(logger, "missing required input: workspace_name")
	}
	workspaceName = strings.TrimSpace(workspaceName)

	rawConfiguration, err := request.RequireString("query_configuration")
	if err != nil || strings.TrimSpace(rawConfiguration) == "" {
		return createQueryToolErrorf(logger, "missing required input: query_configuration")
	}

	configuration, err := parseCreateQueryConfiguration(rawConfiguration)
	if err != nil {
		return createQueryToolErrorf(logger, "invalid query_configuration: %v", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return createQueryToolErrorf(logger, "failed to get Terraform client: %v", err)
	}
	workspace, err := tfeClient.Workspaces.Read(ctx, organizationName, workspaceName)
	if err != nil {
		return createQueryToolErrorf(logger, "workspace %q not found in organization %q: %v", workspaceName, organizationName, err)
	}

	response, err := submitCreateQuery(ctx, tfeClient, workspace.ID, configuration)
	if err != nil {
		return createQueryToolErrorf(logger, "failed to create no-code query: %v", err)
	}

	return mcp.NewToolResultText(response), nil
}

func parseCreateQueryConfiguration(raw string) (*createQueryConfiguration, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var configuration createQueryConfiguration
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

func submitCreateQuery(ctx context.Context, tfeClient *tfe.Client, workspaceID string, configuration *createQueryConfiguration) (string, error) {
	options := &createQueryOptions{
		GenerateConfigOut: configuration.GenerateConfigOut,
		Providers:         configuration.Providers,
		Workspace: &tfe.Workspace{
			ID: workspaceID,
		},
	}

	request, err := tfeClient.NewRequest(http.MethodPost, "search/no-code-query", options)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}

	var response bytes.Buffer
	if err := request.Do(ctx, &response); err != nil {
		return "", err
	}
	return response.String(), nil
}

func createQueryToolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("create_query: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}

const createQueryDescription = `Creates and immediately executes an HCP Terraform no-code Search query.

Pass an organization name, workspace name, and the provider/resource configuration produced
from the guidance returned by generate_query_configuration. The tool resolves the workspace
with go-tfe, constructs the JSON:API request, and
calls POST /api/v2/search/no-code-query, which persists the selections, uploads generated
Terraform query configuration, and starts a query run. It does not create or modify
infrastructure.

The response includes data.relationships.latest-query-run.data.id. Use that query
run ID with the HCP Terraform query APIs to monitor execution and retrieve results.

Requires an authenticated HCP Terraform session, the NO_CODE_QUERY feature for the
workspace organization, and a workspace using a supported Terraform version.`
