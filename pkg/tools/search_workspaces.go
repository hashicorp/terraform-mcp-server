// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SearchWorkspaces creates a tool to search for Terraform workspaces.
func SearchWorkspaces(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_workspaces",
			mcp.WithDescription(`This tool searches for Terraform workspaces in your Terraform Cloud/Enterprise organization. It will list all the workspaces if no search criteria are specified.`),
			mcp.WithTitleAnnotation("Search for Terraform workspaces"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			utils.WithPagination(),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("search_query",
				mcp.Description("Optional search query to filter workspaces by name"),
			),
			mcp.WithString("tags",
				mcp.Description("Optional comma-separated list of tags to filter workspaces"),
			),
			mcp.WithString("exclude_tags",
				mcp.Description("Optional comma-separated list of tags to exclude from results"),
			),
			mcp.WithString("wildcard_name",
				mcp.Description("Optional wildcard pattern to match workspace names"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return searchTerraformWorkspacesHandler(ctx, request, logger)
		},
	}
}

func searchTerraformWorkspacesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get Terraform org name
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required for the Terraform Cloud/Enterprise organization.", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	// Get optional parameters
	searchQuery := request.GetString("search_query", "")
	tagsStr := request.GetString("tags", "")
	excludeTagsStr := request.GetString("exclude_tags", "")
	wildcardName := request.GetString("wildcard_name", "")

	// Parse tags
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(strings.TrimSpace(tagsStr), ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	var excludeTags []string
	if excludeTagsStr != "" {
		excludeTags = strings.Split(strings.TrimSpace(excludeTagsStr), ",")
		for i, tag := range excludeTags {
			excludeTags[i] = strings.TrimSpace(tag)
		}
	}

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", err)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	workspaces, err := tfeClient.Workspaces.List(ctx, terraformOrgName, &tfe.WorkspaceListOptions{
		Search:       searchQuery,
		Tags:         strings.Join(tags, ","),
		ExcludeTags:  strings.Join(excludeTags, ","),
		WildcardName: wildcardName,
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})

	if err != nil {
		return nil, utils.LogAndReturnError(logger, "listing Terraform workspaces", err)
	}

	// Build a formatted string with basic workspace information
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Searching workspaces in organization: %s\n\n", terraformOrgName))
	if len(workspaces.Items) == 0 {
		result.WriteString("No workspaces found matching the search criteria.\n")
	} else {
		result.WriteString(fmt.Sprintf("Found %d workspace(s):\n\n", len(workspaces.Items)))

		for i, workspace := range workspaces.Items {
			result.WriteString(fmt.Sprintf("%d. Workspace: %s\n", i+1, workspace.Name))
			result.WriteString(fmt.Sprintf("   ID: %s\n", workspace.ID))

			if workspace.Description != "" {
				result.WriteString(fmt.Sprintf("   Description: %s\n", workspace.Description))
			}

			if len(workspace.TagNames) > 0 {
				result.WriteString(fmt.Sprintf("   Tags: %s\n", strings.Join(workspace.TagNames, ", ")))
			}

			if workspace.Project != nil {
				result.WriteString(fmt.Sprintf("   Project ID: %s\n", workspace.Project.ID))
			}

			if workspace.SourceURL != "" {
				result.WriteString(fmt.Sprintf("   Source URL: %s\n", workspace.SourceURL))
			}

			if i < len(workspaces.Items)-1 {
				result.WriteString("\n")
			}
		}
	}

	return mcp.NewToolResultText(result.String()), nil
}
