// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DeleteWorkspaceSafelyArguments holds the input parameters for safely deleting a workspace.
type DeleteWorkspaceSafelyArguments struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"The ID of the workspace to delete (for example, ws-abc123def456)"`
}

func DeleteWorkspaceSafelyTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "delete_workspace_safely",
		Description: "Safely deletes a Terraform workspace by ID only if it is not managing any resources. This prevents accidental deletion of workspaces that still have active infrastructure.",
		Annotations: &mcp.ToolAnnotations{Title: "Safely delete a Terraform workspace by ID", OpenWorldHint: ptr(true), ReadOnlyHint: false, DestructiveHint: ptr(true)},
	}
}

func DeleteWorkspaceSafelyFunc(ctx context.Context, _ *mcp.CallToolRequest, input DeleteWorkspaceSafelyArguments) (*mcp.CallToolResult, *WorkspaceMutationResult, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return nil, nil, fmt.Errorf("workspace_id is required")
	}
	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace not found: %s: %w", workspaceID, err)
	}
	if err := tfeClient.Workspaces.SafeDeleteByID(ctx, workspaceID); err != nil {
		return nil, nil, fmt.Errorf("failed to delete workspace %q; it may still have managed resources: %w", workspaceID, err)
	}
	return nil, &WorkspaceMutationResult{
		WorkspaceID:   workspace.ID,
		WorkspaceName: workspace.Name,
		Message:       "Workspace deleted successfully",
	}, nil
}
