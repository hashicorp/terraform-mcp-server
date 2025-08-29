// Copyright (c) HashiCorp, Inc.
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
		return nil, utils.LogAndReturnError(logger, "required input: terraform_org_name is required", err)
	}
	if terraformOrgName == "" {
		return nil, utils.LogAndReturnError(logger, "required input: terraform_org_name cannot be empty", nil)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}
	if tfeClient == nil {
		return nil, utils.LogAndReturnError(logger, "getting TFE client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", nil)
	}

	// Fetch the list of projects
	projects, err := tfeClient.Projects.List(ctx, terraformOrgName, &tfe.ProjectListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})

	if err != nil {
		return nil, utils.LogAndReturnError(logger, "listing Terraform projects, check if the organization exists and you have access", err)
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
		return nil, utils.LogAndReturnError(logger, "marshalling project infos", err)
	}

	return mcp.NewToolResultText(string(projectJSON)), nil
}
