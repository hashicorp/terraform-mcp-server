// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ForceUnlockWorkspace creates a tool to force-unlock a Terraform workspace by ID.
//
// Force-unlock is intended as a recovery action when a workspace lock is stuck
// (for example, after a run was interrupted in a way that left the lock held).
// It should be used with caution: forcing a lock release while a run is still
// in progress can leave the workspace state in an inconsistent condition.
func ForceUnlockWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("force_unlock_workspace",
			mcp.WithDescription(`Force-unlocks a Terraform workspace by ID. Use this to recover a workspace whose lock is stuck (for example after an interrupted run). This is a destructive operation: forcing a lock release while a run is still active can leave the workspace state in an inconsistent condition. Prefer running the responsible run to completion, cancelling it, or using the workspace owner's regular unlock before resorting to force-unlock.`),
			mcp.WithTitleAnnotation("Force-unlock a Terraform workspace by ID"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The ID of the workspace to force-unlock (e.g., 'ws-abc123def456')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return forceUnlockWorkspaceHandler(ctx, request, logger)
		},
	}
}

func forceUnlockWorkspaceHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	workspaceID, err := request.RequireString("workspace_id")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_id", err)
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ToolErrorf(logger, "workspace_id must not be empty")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	workspace, err := tfeClient.Workspaces.ForceUnlock(ctx, workspaceID)
	if err != nil {
		return ToolErrorf(logger, "failed to force-unlock workspace '%s': %v", workspaceID, err)
	}

	result := map[string]interface{}{
		"success":        true,
		"message":        "Workspace force-unlocked successfully.",
		"workspace_id":   workspace.ID,
		"workspace_name": workspace.Name,
		"locked":         workspace.Locked,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal result", err)
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}
