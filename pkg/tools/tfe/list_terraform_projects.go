// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListTerraformProjects creates a tool to get terraform projects.
func ListTerraformProjects(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_terraform_projects",
			mcp.WithDescription(`Fetches a list of all Terraform projects.`),
			mcp.WithTitleAnnotation("List all Terraform projects"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The name of the Terraform organization to list projects for."),
			),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listTerraformProjectsHandler(ctx, req, logger)
		},
	}
}

func listTerraformProjectsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	if terraformOrgName == "" {
		return ToolError(logger, "terraform_org_name cannot be empty", nil)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "invalid pagination parameters", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	projects, err := tfeClient.Projects.List(ctx, terraformOrgName, &tfe.ProjectListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})
	if err != nil {
		return ToolErrorf(logger, "failed to list projects in org '%s' - check if the organization exists and you have access", terraformOrgName)
	}

	projectInfos := make([]map[string]string, 0, len(projects.Items))
	for _, project := range projects.Items {
		projectInfos = append(projectInfos, map[string]string{
			"project_name": project.Name,
			"project_id":   project.ID,
		})
	}

	projectJSON, err := json.Marshal(projectInfos)
	if err != nil {
		return ToolError(logger, "failed to marshal project infos", err)
	}

	return mcp.NewToolResultText(string(projectJSON)), nil
}
