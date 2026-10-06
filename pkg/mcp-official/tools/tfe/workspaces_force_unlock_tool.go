// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForceUnlockWorkspaceArguments holds the input parameters for force-unlocking a workspace.
type ForceUnlockWorkspaceArguments struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"The ID of the workspace to force unlock (e.g. 'ws-abc123def456')."`
}

// ForceUnlockWorkspaceResponse is the response shape returned by the force_unlock_workspace tool.
type ForceUnlockWorkspaceResponse struct {
	WorkspaceID string `json:"workspace_id"`
	Unlocked    bool   `json:"unlocked"`
}

// ForceUnlockWorkspaceTool describes the force_unlock_workspace tool.
func ForceUnlockWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "force_unlock_workspace",
		Description: `Force unlocks a Terraform workspace stuck in a lock. Prefer using the action_run tool with "discard" or "cancel" before force-unlocking a workspace. Requires workspace admin permissions (e.g. an Owners team token).`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Force unlock a Terraform workspace by ID",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func ForceUnlockWorkspaceFunc(ctx context.Context, request *mcp.CallToolRequest, input ForceUnlockWorkspaceArguments) (*mcp.CallToolResult, *ForceUnlockWorkspaceResponse, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return nil, nil, fmt.Errorf("workspace_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	// Verify the workspace exists before attempting the unlock.
	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}

	// Reject early if the workspace is not locked, to avoid a misleading
	// "resource not found" error from the TFE API.
	if !workspace.Locked {
		return nil, nil, fmt.Errorf("workspace %q is not locked", workspaceID)
	}

	if _, err := tfeClient.Workspaces.ForceUnlock(ctx, workspaceID); err != nil {
		return nil, nil, fmt.Errorf("failed to force unlock workspace %q: %w", workspaceID, err)
	}

	return nil, &ForceUnlockWorkspaceResponse{WorkspaceID: workspaceID, Unlocked: true}, nil
}
