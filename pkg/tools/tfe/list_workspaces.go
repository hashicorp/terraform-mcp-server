// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ListWorkspaces creates a tool to list Terraform workspaces.
func ListWorkspaces(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_workspaces",
			mcp.WithDescription(`Search and list Terraform workspaces within a specified organization. Returns all workspaces when no filters are applied, or filters results based on name patterns, tags, or search queries. Supports pagination for large result sets.`),
			mcp.WithTitleAnnotation("List Terraform workspaces with queries"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			utils.WithPagination(),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform organization name"),
			),
			mcp.WithString("search_query",
				mcp.Description("Optional search query to filter workspaces by name"),
			),
			mcp.WithString("project_id",
				mcp.Description("Optional project ID to filter workspaces"),
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
	projectID := request.GetString("project_id", "")
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
		ProjectID:    projectID,
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

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, workspaces.Items)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling workspace creation result", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
