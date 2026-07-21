// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetCurrentWorkspaceStateVersion creates a tool to get latest available state from the given workspace.
func GetCurrentWorkspaceStateVersion(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"get_current_workspace_state_version",
			mcp.WithDescription("Gets latest available state from the given workspace ID"),
			mcp.WithTitleAnnotation("Gets State-Version with Workspace ID"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("workspace-id",
				mcp.Required(),
				mcp.Description("The Workspace id"),
			),
		),

		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getCurrentWorkspaceStateVersionHandler(ctx, req, logger)
		},
	}

}

// getCurrentWorkspaceStateVersionHandler handles tool logics and functionality
func getCurrentWorkspaceStateVersionHandler(ctx context.Context,
	request mcp.CallToolRequest,
	logger *log.Logger) (*mcp.CallToolResult, error) {

	// init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)

	// Failed to get Terraform client
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// client context nill
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Required params
	workspaceID, err := request.RequireString("workspace-id")
	if err != nil {
		return ToolError(logger, "missing required input: workspace-id", err)
	}
	workspaceID = strings.TrimSpace(workspaceID)

	// Check if input empty
	if workspaceID == "" {
		return ToolError(logger, "workspace-id cannot be empty", nil)
	}

	// Get State Version
	ws, err := tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)

	// If tool fails
	if err != nil {
		return ToolError(logger, "failed to get workspace's state versions", err)
	}

	// Serialize -> Marshal JSON
	wsJSON, err := json.Marshal(ws)

	if err != nil {
		return ToolError(logger, "failed to serialize state version", err)
	}

	return mcp.NewToolResultText(string(wsJSON)), nil

}
