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

// DeleteProjectArguments holds the input parameters for deleting a project.
type DeleteProjectArguments struct {
	// Required field
	ProjectID string `json:"project_id" jsonschema:"The ID of the project to delete (e.g., 'prj-abc123def456')"`
}

// DeleteProjectResponse is the response shape returned by the delete_project tool.
type DeleteProjectResponse struct {
	ID      string `json:"project_id"`
	Deleted bool   `json:"deleted"`
}

// DeleteProjectTool describes the delete_project tool. TFC/TFE rejects the
// request if the project still contains workspaces or stacks.
func DeleteProjectTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "delete_project",
		Description: `Deletes a Terraform project by ID. This is a destructive operation. The request will fail if the project still contains workspaces or stacks. If the project ID isn't already known, call list_terraform_projects first to look it up rather than asking the user to find it themselves.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete a Terraform project by ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
		},
	}
}

func DeleteProjectFunc(ctx context.Context, request *mcp.CallToolRequest, input DeleteProjectArguments) (*mcp.CallToolResult, *DeleteProjectResponse, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, nil, fmt.Errorf("project_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if err := tfeClient.Projects.Delete(ctx, projectID); err != nil {
		return nil, nil, fmt.Errorf("deleting project %q: %w", projectID, err)
	}

	return nil, &DeleteProjectResponse{ID: projectID, Deleted: true}, nil
}
