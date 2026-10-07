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

// DeleteWorkspaceSafelyArguments holds the workspace ID to delete.
type DeleteWorkspaceSafelyArguments struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"The ID of the workspace to delete (e.g., 'ws-abc123def456')"`
}

// DeleteWorkspaceSafelyResponse reports the workspace deleted by the tool.
type DeleteWorkspaceSafelyResponse struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Deleted       bool   `json:"deleted"`
}

// DeleteWorkspaceSafelyTool describes the delete_workspace_safely tool.
func DeleteWorkspaceSafelyTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "delete_workspace_safely",
		Description: "Safely deletes a Terraform workspace by ID only if it is not managing any resources.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Safely delete a Terraform workspace by ID",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func DeleteWorkspaceSafelyFunc(ctx context.Context, request *mcp.CallToolRequest, input DeleteWorkspaceSafelyArguments) (*mcp.CallToolResult, *DeleteWorkspaceSafelyResponse, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return nil, nil, fmt.Errorf("workspace_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading workspace %q: %w", workspaceID, err)
	}

	if err := tfeClient.Workspaces.SafeDeleteByID(ctx, workspaceID); err != nil {
		return nil, nil, fmt.Errorf("safely deleting workspace %q: %w", workspaceID, err)
	}

	return nil, &DeleteWorkspaceSafelyResponse{
		WorkspaceID:   workspaceID,
		WorkspaceName: workspace.Name,
		Deleted:       true,
	}, nil
}
