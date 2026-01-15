// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
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
			mcp.WithDescription(`Search and list Terraform workspaces within a specified organization. Returns all workspaces when no filters are applied, or filters results based on name patterns, tags, or search queries. Supports pagination for large result sets. Returns a truncated summary of the workspace, use get_workspace_details to get the full details for a specific workspace.`),
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
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	projectID := request.GetString("project_id", "")
	searchQuery := request.GetString("search_query", "")
	tagsStr := request.GetString("tags", "")
	excludeTagsStr := request.GetString("exclude_tags", "")
	wildcardName := request.GetString("wildcard_name", "")

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

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "invalid pagination parameters", err)
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
		return ToolErrorf(logger, "failed to list workspaces in org '%s'", terraformOrgName)
	}
	if len(workspaces.Items) == 0 {
		return ToolErrorf(logger, "no workspaces to list in organization %q", terraformOrgName)
	}

	summaries := make([]*WorkspaceSummary, len(workspaces.Items))
	for i, w := range workspaces.Items {
		summaries[i] = &WorkspaceSummary{
			ID:            w.ID,
			Name:          w.Name,
			Description:   w.Description,
			Environment:   w.Environment,
			CreatedAt:     w.CreatedAt,
			ExecutionMode: w.ExecutionMode,
		}
	}

	buf, err := json.Marshal(&WorkspaceSummaryList{
		Items:      summaries,
		Pagination: workspaces.Pagination,
	})
	if err != nil {
		return ToolError(logger, "failed to marshal workspaces", err)
	}

	return mcp.NewToolResultText(string(buf)), nil
}

// WorkspaceSummary is a truncated summary of a Workspace for top level listing
type WorkspaceSummary struct {
	ID            string    `json:"id"`
	Name          string    `json:"workspace_name"`
	Description   string    `json:"description"`
	Environment   string    `json:"environment"`
	CreatedAt     time.Time `json:"created_at"`
	ExecutionMode string    `json:"execution_mode"`
}

// WorkspaceSummaryList contains the list of workspace summaries and pagination details
type WorkspaceSummaryList struct {
	Items []*WorkspaceSummary `json:"items"`
	*tfe.Pagination
}
