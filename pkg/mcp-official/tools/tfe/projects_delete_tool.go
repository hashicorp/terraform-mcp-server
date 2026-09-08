// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// DeleteProjectArguments holds the input parameters for deleting a project.
type DeleteProjectArguments struct {
	// Required field
	ProjectID string `json:"project_id"`
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
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"project_id": {
					Type:        "string",
					Description: "The ID of the project to delete (e.g., 'prj-abc123def456')",
				},
			},
			PropertyOrder:        []string{"project_id"},
			Required:             []string{"project_id"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete a Terraform project by ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
		},
	}
}

func DeleteProjectFunc(logger *log.Logger) mcp.ToolHandlerFor[DeleteProjectArguments, *DeleteProjectResponse] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input DeleteProjectArguments) (*mcp.CallToolResult, *DeleteProjectResponse, error) {
		projectID := strings.TrimSpace(input.ProjectID)
		if projectID == "" {
			return nil, nil, toolError(logger, "project_id must not be blank", nil)
		}

		tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
		if err != nil {
			return nil, nil, toolError(logger, "getting Terraform client", err)
		}

		if err := tfeClient.Projects.Delete(ctx, projectID); err != nil {
			return nil, nil, toolError(logger, "deleting project "+projectID, err)
		}

		return nil, &DeleteProjectResponse{ID: projectID, Deleted: true}, nil
	}
}
