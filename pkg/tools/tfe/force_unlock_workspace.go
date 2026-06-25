package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func ForceUnlockWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("force_unlock_workspace",
				mcp.WithDescription(`Force unlocks a Terraform workspace whose lock has become stuck — for example when a run crashed, was interrupted, or timed out without releasing the lock. Use this only as a last resort: if the locking run is still active, prefer cancelling or discarding it first via the action_run tool, because force-unlocking while a run is in progress can leave the workspace state inconsistent. Requires a token with workspace admin permissions (e.g. an Owners team token with the 'Destroy' and 'Update' permissions). This is a destructive operation and is gated behind ENABLE_TF_OPERATIONS=true.`),
				mcp.WithTitleAnnotation("Force unlock a Terraform workspace by ID"),
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(true),
				mcp.WithString("workspace_id",
					mcp.Required(),
					mcp.Description("he ID of the workspace to force unlock (e.g. 'ws-abc123def456'). Must be locked by a run, not manually locked by a user.`"),
				),
			),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return forceUnlockWorkspace(ctx, request, logger)
		},
	}
}

func forceUnlockWorkspace(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error){
	workspaceID, err := request.RequireString("workspace_id")

	if err != nil{
		return ToolError(logger, "missing required input: workspace_id", err)
	}

	workspaceID = strings.TrimSpace(workspaceID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)

	if err != nil{
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)

	if err != nil{
		return ToolErrorf(logger, "workspace not found: %s", workspaceID)
	}

	if !workspace.Locked{
		return ToolErrorf(logger, "workspace '%s' is not locked", workspaceID)
	}

	workspace, err = tfeClient.Workspaces.ForceUnlock(ctx, workspaceID)

	if err != nil{
		return ToolErrorf(logger, "failed to force unlock workspace '%s'. This is the reported error: %v", workspaceID, err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "force_unlock_workspace", tfeClient, workspace, logger)

	if err != nil {
		return ToolError(logger, "failed to get workspace details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}