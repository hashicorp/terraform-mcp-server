// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
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
	// Get required parameters
	workspaceID, err := request.RequireString("workspace_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'workspace_id' parameter is required", err)
	}
	workspaceID = strings.TrimSpace(workspaceID)

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", err)
	}

	// First, get the workspace details to check its current state
	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace details", err)
	}

	// Perform the deletion using workspace ID
	err = tfeClient.Workspaces.SafeDeleteByID(ctx, workspaceID)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "deleting workspace", err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "delete_workspace_safely", tfeClient, workspace, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting workspace details for tools", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
