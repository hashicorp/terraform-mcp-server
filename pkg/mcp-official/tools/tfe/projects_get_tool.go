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

// GetProjectArguments holds the input parameters for fetching a single project.
type GetProjectArguments struct {
	// Required field
	ProjectID string `json:"project_id" jsonschema:"The ID of the project to fetch (e.g., 'prj-abc123def456')"`
}

func GetProjectTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_project",
		Description: `Fetches detailed information about a Terraform project by its ID. If the project ID isn't already known, call "list_terraform_projects" first.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get a Terraform project by ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetProjectFunc(ctx context.Context, request *mcp.CallToolRequest, input GetProjectArguments) (*mcp.CallToolResult, *ProjectDetails, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, nil, fmt.Errorf("project_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading project %q: %w", projectID, err)
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

	return nil, details, nil
}
