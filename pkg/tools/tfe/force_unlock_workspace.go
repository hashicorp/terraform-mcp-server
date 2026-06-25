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
// Gated behind ENABLE_TF_OPERATIONS=true, same as delete_workspace_safely and action_run.
func ForceUnlockWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("force_unlock_workspace",
			mcp.WithDescription(`Force unlocks a Terraform workspace stuck in a run-held lock. Prefer cancelling or discarding the active run via action_run first — force-unlocking while a run is in progress can corrupt state. Requires workspace admin permissions (e.g. an Owners team token). Gated behind ENABLE_TF_OPERATIONS=true.`),
			mcp.WithTitleAnnotation("Force unlock a Terraform workspace by ID"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The ID of the workspace to force unlock (e.g. 'ws-abc123def456'). Must be locked by a run, not manually locked by a user."),
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

	// Guard: ForceUnlock only applies to run-held locks. Reject early if the
	// workspace is not locked to avoid a misleading "resource not found" from
	// the TFE API.
	if !workspace.Locked {
		return ToolErrorf(logger, "workspace '%s' is not locked", workspaceID)
	}

	// Perform the force unlock. This calls POST /workspaces/:id/actions/force-unlock
	// and requires workspace admin permissions (e.g. an Owners team token).
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
