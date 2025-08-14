// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/orchestrator"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// WorkspaceOrchestrator creates the MCP tool for comprehensive workspace analysis
func WorkspaceOrchestrator(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_workspace_comprehensive_analysis",
			Description: "Performs comprehensive workspace analysis including details, variables, configurations, and state information",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to analyze (use this OR organization_name + workspace_name)",
					},
					"organization_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the organization (required if using workspace_name instead of workspace_id)",
					},
					"workspace_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the workspace to analyze (required if using organization_name instead of workspace_id)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return workspaceOrchestratorHandler(hcpClient, request, logger)
		},
	}
}

// workspaceOrchestratorHandler handles comprehensive workspace analysis requests
func workspaceOrchestratorHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Parse request parameters
	req := &WorkspaceAnalysisRequest{
		WorkspaceID:      request.GetString("workspace_id", ""),
		OrganizationName: request.GetString("organization_name", ""),
		WorkspaceName:    request.GetString("workspace_name", ""),
		Authorization:    request.GetString("authorization", ""),
	}

	// Perform workspace analysis
	workspaceInfo, err := analyzeWorkspace(hcpClient, req, logger)
	if err != nil {
		return nil, err
	}

	// Return the analysis results
	result, err := json.Marshal(workspaceInfo)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []interface{}{
			mcp.TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}

// ConfigurationPreparator creates the MCP tool for configuration preparation
func ConfigurationPreparator(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "prepare_workspace_configuration",
			Description: "Prepares Terraform configuration for workspace replication by adding tags and modifying provider settings",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"configuration_archive": map[string]interface{}{
						"type":        "string",
						"description": "Base64-encoded tar.gz archive containing Terraform configuration files",
					},
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "Target workspace ID for replication",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication",
					},
				},
				Required: []string{"configuration_archive", "workspace_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return configurationPreparatorHandler(request, logger)
		},
	}
}

// configurationPreparatorHandler handles configuration preparation requests
func configurationPreparatorHandler(request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Parse request parameters
	configArchive := request.GetString("configuration_archive", "")
	workspaceID := request.GetString("workspace_id", "")
	authorization := request.GetString("authorization", "")

	// Create configuration preparation request
	prepRequest := &orchestrator.ConfigPreparationRequest{
		ConfigurationArchive: configArchive,
		WorkspaceID:          workspaceID,
		Authorization:        authorization,
		TagUpdates:           make(map[string]string),
		VariableUpdates:      make(map[string]string),
		ProviderUpdates:      make(map[string]interface{}),
	}

	// Create orchestrator and prepare configuration
	orch := orchestrator.NewOrchestrator(logger)
	response, err := orch.PrepareConfiguration(prepRequest)
	if err != nil {
		return nil, err
	}

	// Return the prepared configuration
	result, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		Content: []interface{}{
			mcp.TextContent{
				Type: "text",
				Text: string(result),
			},
		},
	}, nil
}
