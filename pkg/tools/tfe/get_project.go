// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetProject creates a tool to fetch a Terraform project by its ID.
func GetProject(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"get_project",
			mcp.WithDescription(`Fetches detailed information about a Terraform project by its ID. If the project ID isn't already known, call "list_terraform_projects" first.`),
			mcp.WithTitleAnnotation("Get a Terraform project by ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("The ID of the project to fetch (e.g., 'prj-abc123def456')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getProjectHandler(ctx, request, logger)
		},
	}
}

// getProjectHandler handles tool logics and functionality
func getProjectHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return ToolError(logger, "Missing required input: project_id", err)
	}
	projectID = strings.TrimSpace(projectID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return ToolErrorf(logger, "Failed to read project %q: %v", projectID, err)
	}

	details := &ProjectDetails{
		ID:                   project.ID,
		Name:                 project.Name,
		Description:          project.Description,
		DefaultExecutionMode: project.DefaultExecutionMode,
		IsUnified:            project.IsUnified,
	}
	if project.Organization != nil {
		details.OrganizationName = project.Organization.Name
	}
	if project.DefaultAgentPool != nil {
		details.DefaultAgentPoolID = project.DefaultAgentPool.ID
		details.DefaultAgentPoolName = project.DefaultAgentPool.Name
	}
	if project.AutoDestroyActivityDuration.IsSpecified() && !project.AutoDestroyActivityDuration.IsNull() {
		if v, err := project.AutoDestroyActivityDuration.Get(); err == nil {
			details.AutoDestroyActivityDuration = v
		}
	}

	projectJSON, err := json.Marshal(details)
	if err != nil {
		return ToolError(logger, "Failed to serialize project", err)
	}
	return mcp.NewToolResultText(string(projectJSON)), nil
}

// ProjectDetails is the response shape returned by the get_project tool.
type ProjectDetails struct {
	ID                          string `json:"project_id"`
	Name                        string `json:"project_name"`
	Description                 string `json:"description,omitempty"`
	DefaultExecutionMode        string `json:"default_execution_mode,omitempty"`
	IsUnified                   bool   `json:"is_unified"`
	AutoDestroyActivityDuration string `json:"auto_destroy_activity_duration,omitempty"`
	OrganizationName            string `json:"organization_name,omitempty"`
	DefaultAgentPoolID          string `json:"default_agent_pool_id,omitempty"`
	DefaultAgentPoolName        string `json:"default_agent_pool_name,omitempty"`
}
