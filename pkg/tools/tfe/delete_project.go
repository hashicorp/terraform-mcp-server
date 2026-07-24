// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DeleteProject creates a tool to delete a Terraform project by ID.
// TFC/TFE will reject the request if the project still contains workspaces or stacks.
func DeleteProject(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_project",
			mcp.WithDescription(`Deletes a Terraform project by ID. This is a destructive operation. The request will fail if the project still contains workspaces or stacks.`),
			mcp.WithTitleAnnotation("Delete a Terraform project by ID"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("The ID of the project to delete (e.g., 'prj-abc123def456')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return deleteProjectHandler(ctx, request, logger)
		},
	}
}

func deleteProjectHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return ToolError(logger, "missing required input: project_id", err)
	}
	projectID = strings.TrimSpace(projectID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	err = tfeClient.Projects.Delete(ctx, projectID)
	if err != nil {
		return ToolErrorf(logger, "failed to delete project '%s': %v", projectID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf("project %s deleted", projectID)), nil
}
