// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"slices"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

var validExecutionModes = []string{"local", "agent", "remote"}

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
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization to create the project in"`
	ProjectName      string `json:"project_name" jsonschema:"The project name. Must be 3-40 characters and may contain letters, numbers, spaces, hyphens, and underscores. It cannot start or end with a space."`

	// Optional fields (will be empty strings if not provided)
	Description          string `json:"description,omitempty" jsonschema:"Optional project description. Must be no more than 256 characters"`
	DefaultExecutionMode string `json:"default_execution_mode,omitempty" jsonschema:"Optional default execution mode for workspaces in the project: local, agent, remote. If not set, workspaces inherit the organization's default execution mode."`
}

func CreateProjectTool() *mcp.Tool {
	// The `jsonschema` struct tag can only set a property description, so the
	// length, pattern and enum constraints are patched onto the inferred schema.
	schema := inferSchema[CreateProjectArguments]("create_project")
	schema.Properties["project_name"].MinLength = ptr(3)
	schema.Properties["project_name"].MaxLength = ptr(40)
	schema.Properties["project_name"].Pattern = `^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$`
	schema.Properties["description"].MaxLength = ptr(256)
	schema.Properties["default_execution_mode"].Enum = enumOf(validExecutionModes...)

	return &mcp.Tool{
		Name:        "create_project",
		Description: "Creates a new Terraform project in the specified organization.",
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform project",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func CreateProjectFunc(logger *log.Logger) mcp.ToolHandlerFor[CreateProjectArguments, *CreateProjectResponse] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input CreateProjectArguments) (*mcp.CallToolResult, *CreateProjectResponse, error) {
		terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
		projectName := strings.TrimSpace(input.ProjectName)

		if terraformOrgName == "" {
			return nil, nil, toolError(logger, "terraform_org_name must not be blank", nil)
		}
		if projectName == "" {
			return nil, nil, toolError(logger, "project_name must not be blank", nil)
		}

		options := tfe.ProjectCreateOptions{Name: projectName}

		if description := strings.TrimSpace(input.Description); description != "" {
			options.Description = &description
		}

		// Defence in depth: the schema enum rejects unknown values before the handler
		// runs, but keep the check so the tool stays correct if the schema changes.
		if mode := strings.ToLower(strings.TrimSpace(input.DefaultExecutionMode)); mode != "" {
			if !slices.Contains(validExecutionModes, mode) {
				return nil, nil, toolError(logger, "invalid default_execution_mode "+input.DefaultExecutionMode+
					": must be one of "+strings.Join(validExecutionModes, ", "), nil)
			}
			options.DefaultExecutionMode = tfe.String(mode)
		}

		tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
		if err != nil {
			return nil, nil, toolError(logger, "getting Terraform client", err)
		}

		project, err := tfeClient.Projects.Create(ctx, terraformOrgName, options)
		if err != nil {
			return nil, nil, toolError(logger, "creating project "+projectName+" in organization "+terraformOrgName, err)
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
}
