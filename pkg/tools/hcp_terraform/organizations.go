// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"context"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetOrganizations creates the MCP tool for listing organizations
func GetOrganizations(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_organizations",
			Description: "Fetches all organizations from HCP Terraform that the authenticated user has access to",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Optional search query to filter organizations by name or email",
					},
					"query_email": map[string]interface{}{
						"type":        "string",
						"description": "Optional search query to filter organizations by notification email",
					},
					"query_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional search query to filter organizations by name",
					},
					"page_size": map[string]interface{}{
						"type":        "integer",
						"description": "Number of organizations per page (default: 20, max: 100)",
						"minimum":     1,
						"maximum":     100,
						"default":     20,
					},
					"page_number": map[string]interface{}{
						"type":        "integer",
						"description": "Page number to retrieve (default: 1)",
						"minimum":     1,
						"default":     1,
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getOrganizationsHandler(hcpClient, request, logger)
		},
	}
}

func getOrganizationsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Parse request parameters
	opts := &hcp_terraform.OrganizationListOptions{
		PageSize:   request.GetInt("page_size", 20),
		PageNumber: request.GetInt("page_number", 1),
	}

	// Add optional query parameters
	if query := request.GetString("query", ""); query != "" {
		opts.Query = query
	}
	if queryEmail := request.GetString("query_email", ""); queryEmail != "" {
		opts.QueryEmail = queryEmail
	}
	if queryName := request.GetString("query_name", ""); queryName != "" {
		opts.QueryName = queryName
	}

	logger.Debugf("Fetching HCP Terraform organizations with options: %+v", opts)

	// Fetch organizations
	response, err := hcpClient.GetOrganizations(token, opts)
	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform organizations: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched %d organizations", len(response.Data))

	// Format response
	result := formatOrganizationsResponse(response)
	return mcp.NewToolResultText(result), nil
}
