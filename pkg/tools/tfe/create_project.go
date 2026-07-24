// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

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
			mcp.WithDescription(`Creates a new Terraform project in the specified organization. Projects are used to group and organize workspaces within an organization.`),
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
				mcp.Description("The name of the project to create"),
			),
			mcp.WithString("description",
				mcp.Description("Optional description for the project"),
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
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	projectName, err := request.RequireString("project_name")
	if err != nil {
		return ToolError(logger, "missing required input: project_name", err)
	}
	projectName = strings.TrimSpace(projectName)

	description := request.GetString("description", "")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
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

	result := struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description,omitempty"`
		Organization string `json:"organization"`
	}{
		ID:           project.ID,
		Name:         project.Name,
		Description:  project.Description,
		Organization: terraformOrgName,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal project result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}
