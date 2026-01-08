// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DeleteWorkspaceSafely creates a tool to safely delete a Terraform workspace by ID.
// It will only delete the workspace if it has no managed resources.
func DeleteWorkspaceSafely(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_workspace_safely",
			mcp.WithDescription(`Safely deletes a Terraform workspace by ID only if it is not managing any resources. This prevents accidental deletion of workspaces that still have active infrastructure. This is a destructive operation.`),
			mcp.WithTitleAnnotation("Safely delete a Terraform workspace by ID"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The ID of the workspace to delete (e.g., 'ws-abc123def456')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return deleteWorkspaceSafelyHandler(ctx, request, logger)
		},
	}
}

func deleteWorkspaceSafelyHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	workspaceID, err := request.RequireString("workspace_id")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_id", err)
	}
	workspaceID = strings.TrimSpace(workspaceID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return ToolErrorf(logger, "workspace not found: %s", workspaceID)
	}

	err = tfeClient.Workspaces.SafeDeleteByID(ctx, workspaceID)
	if err != nil {
		return ToolErrorf(logger, "failed to delete workspace '%s' - it may still have managed resources: %v", workspaceID, err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "delete_workspace_safely", tfeClient, workspace, logger)
	if err != nil {
		return ToolError(logger, "failed to get workspace details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
