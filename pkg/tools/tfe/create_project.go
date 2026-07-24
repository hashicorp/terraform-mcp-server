// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// CreateProject creates a tool to create a new Terraform project.
func CreateProject(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_project",
			mcp.WithDescription(`Creates a new Terraform project in the specified organization. Call list_terraform_orgs first, then use ask_followup_question with each org name as a suggestion button so the user can select one without typing. Then ask the user for the project name and if they'd like to add an optional description — wait for their answer before proceeding.`),
			mcp.WithTitleAnnotation("Create a new Terraform project"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The name of the Terraform Cloud/Enterprise organization to create the project in"),
			),
			mcp.WithString("project_name",
				mcp.Required(),
				mcp.Description("The project name. Must be 3-40 characters, contain only letters, numbers, spaces, hyphens, and underscores, and not start or end with a space."),
				mcp.MinLength(3),
				mcp.MaxLength(40),
				mcp.Pattern(`^[A-Za-z0-9_-][A-Za-z0-9_-]*[A-Za-z0-9_-]$`),
			),
			mcp.WithString("description",
				mcp.Description("Optional project description. Must be no more than 256 characters"),
				mcp.MaxLength(256),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createProjectHandler(ctx, request, logger)
		},
	}
}

func createProjectHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}

	projectName, err := request.RequireString("project_name")
	if err != nil {
		return ToolError(logger, "missing required input: project_name", err)
	}

	description := request.GetString("description", "")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	options := tfe.ProjectCreateOptions{
		Name: projectName,
	}

	if description != "" {
		options.Description = &description
	}

	project, err := tfeClient.Projects.Create(ctx, terraformOrgName, options)
	if err != nil {
		return ToolErrorf(logger, "failed to create project '%s' in org '%s': %v", projectName, terraformOrgName, err)
	}

	projectJSON, err := json.Marshal(&ProjectSummary{
		ID:   project.ID,
		Name: project.Name,
	})
	if err != nil {
		return ToolError(logger, "failed to marshal created project summary", err)
	}

	return mcp.NewToolResultText(string(projectJSON)), nil
}
