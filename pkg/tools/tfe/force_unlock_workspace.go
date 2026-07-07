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

// ForceUnlockWorkspace creates a tool to force unlock a Terraform workspace that
// is stuck in a locked state due to a crashed, interrupted, or timed-out run.
func ForceUnlockWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("force_unlock_workspace",
			mcp.WithDescription(`Force unlocks a Terraform workspace stuck in a lock. Prefer using the action_run tool with "discard" or "cancel" before force-unlocking a workspace. Requires workspace admin permissions (e.g. an Owners team token).`),
			mcp.WithTitleAnnotation("Force unlock a Terraform workspace by ID"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The ID of the workspace to force unlock (e.g. 'ws-abc123def456')."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return forceUnlockWorkspace(ctx, request, logger)
		},
	}
}

func forceUnlockWorkspace(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	workspaceID, err := request.RequireString("workspace_id")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_id", err)
	}
	workspaceID = strings.TrimSpace(workspaceID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	// Verify the workspace exists before attempting the unlock.
	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return ToolErrorf(logger, "workspace not found: %s", workspaceID)
	}

	// Guard: Reject early if the
	// workspace is not locked to avoid a misleading "resource not found" from
	// the TFE API.
	if !workspace.Locked {
		return ToolErrorf(logger, "workspace %q is not locked", workspaceID)
	}
	
	workspace, err = tfeClient.Workspaces.ForceUnlock(ctx, workspaceID)
	if err != nil {
		return ToolErrorf(logger, "failed to force unlock workspace '%s'. This is the reported error: %v", workspaceID, err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "force_unlock_workspace", tfeClient, workspace, logger)
	if err != nil {
		return ToolError(logger, "failed to get workspace details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
