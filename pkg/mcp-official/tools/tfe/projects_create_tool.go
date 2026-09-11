// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	executionModeLocal  = "local"
	executionModeAgent  = "agent"
	executionModeRemote = "remote"
)

var validExecutionModes = []string{executionModeLocal, executionModeAgent, executionModeRemote}

// CreateProjectResponse is the response shape returned by the create_project tool.
type CreateProjectResponse struct {
	ID                   string `json:"project_id"`
	Name                 string `json:"project_name"`
	Description          string `json:"description,omitempty"`
	DefaultExecutionMode string `json:"default_execution_mode,omitempty"`
	OrganizationName     string `json:"organization_name,omitempty"`
	IsUnified            bool   `json:"is_unified"`
}

// CreateProjectArguments holds the input parameters for creating a project within an organization.
type CreateProjectArguments struct {
	// Required fields
	TerraformOrgName string `json:"terraform_org_name"`
	ProjectName      string `json:"project_name"`

	// Optional fields (will be empty strings if not provided)
	Description          string `json:"description,omitempty"`
	DefaultExecutionMode string `json:"default_execution_mode,omitempty"`
}

func CreateProjectTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_project",
		Description: "Creates a new Terraform project in the specified organization.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization to create the project in",
				},
				"project_name": {
					Type:        "string",
					Description: "The project name. Must be 3-40 characters and may contain letters, numbers, spaces, hyphens, and underscores. It cannot start or end with a space.",
					MinLength:   ptr(3),
					MaxLength:   ptr(40),
					Pattern:     `^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$`,
				},
				"description": {
					Type:        "string",
					Description: "Optional project description. Must be no more than 256 characters",
					MaxLength:   ptr(256),
				},
				"default_execution_mode": {
					Type:        "string",
					Description: "Optional default execution mode for workspaces in the project: local, agent, remote. If not set, workspaces inherit the organization's default execution mode.",
					Enum:        []any{executionModeLocal, executionModeAgent, executionModeRemote},
				},
			},
			PropertyOrder: []string{
				"terraform_org_name",
				"project_name",
				"description",
				"default_execution_mode",
			},
			Required:             []string{"terraform_org_name", "project_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform project",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func CreateProjectFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateProjectArguments) (*mcp.CallToolResult, *CreateProjectResponse, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	projectName := strings.TrimSpace(input.ProjectName)

	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if projectName == "" {
		return nil, nil, fmt.Errorf("project_name must not be blank")
	}

	options := tfe.ProjectCreateOptions{Name: projectName}

	if description := strings.TrimSpace(input.Description); description != "" {
		options.Description = &description
	}

	if mode := strings.ToLower(strings.TrimSpace(input.DefaultExecutionMode)); mode != "" {
		if !slices.Contains(validExecutionModes, mode) {
			return nil, nil, fmt.Errorf("invalid default_execution_mode %q: must be one of %s",
				input.DefaultExecutionMode, strings.Join(validExecutionModes, ", "))
		}
		options.DefaultExecutionMode = tfe.String(mode)
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	project, err := tfeClient.Projects.Create(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("creating project %q in organization %q: %w", projectName, terraformOrgName, err)
	}

	response := &CreateProjectResponse{
		ID:                   project.ID,
		Name:                 project.Name,
		Description:          project.Description,
		DefaultExecutionMode: project.DefaultExecutionMode,
		IsUnified:            project.IsUnified,
	}
	if project.Organization != nil {
		response.OrganizationName = project.Organization.Name
	}

	return nil, response, nil

}
