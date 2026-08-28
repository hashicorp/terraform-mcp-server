package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//create_project -> WIP

// ProjectResponse is a truncated set of information about a project for listing
// type ProjectResponse struct {
// 	Succeeded string `json:"succeeded"`
// 	Failed    string `json:"failed"`
// }

// CreateProjectArguments holds the input parameters for creating a project within an organization.
type CreateProjectArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	ProjectName      string `json:"project_name" jsonschema:"The project name. Must be 3-40 characters and may contain letters, numbers, spaces, hyphens, and underscores. It cannot start or end with a space."`

	// Optional fields (will be empty strings if not provided)
	Description          string `json:"description,omitempty" jsonschema:"Optional project description. Must be no more than 256 characters"`
	DefaultExecutionMode string `json:"default_execution_mode,omitempty" jsonschema:"Optional default execution mode for workspaces in the project: local, agent, remote. If not set, workspaces inherit the organization's default execution mode."`
}

func CreateProjectTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_project",
		Description: "Creates a new Terraform project in the specified organization.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform project",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func CreateProjectFunc(ctx context.Context, request *mcp.CallToolRequest, input GetProjectsArguments) (*mcp.CallToolResult, *ProjectDetails, error) {
	projectID := strings.TrimSpace(input.ProjectID)

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to read project %q:  %w", projectID, err)
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
