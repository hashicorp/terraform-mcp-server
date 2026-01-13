// Copyright IBM Corp. 2025
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

// ListStacks creates a tool to list Terraform workspaces.
func ListStacks(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_stacks",
			mcp.WithDescription(`List Stacks within a specified organization. Returns all stacks when no project or search query is supplied. Supports pagination for large result sets.`),
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
				mcp.Description("Optional search query to filter stacks by name"),
			),
			mcp.WithString("project_id",
				mcp.Description("Optional project ID to filter stacks"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listTerraformStacksHandler(ctx, request, logger)
		},
	}
}

func listTerraformStacksHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	projectID := request.GetString("project_id", "")
	searchQuery := request.GetString("search_query", "")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "invalid pagination parameters", err)
	}

	stacks, err := tfeClient.Stacks.List(ctx, terraformOrgName, &tfe.StackListOptions{
		ProjectID:    projectID,
		SearchByName: searchQuery,
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})
	if err != nil {
		return ToolErrorf(logger, "failed to list stacks in org %q", terraformOrgName)
	}

	// create list of summaries
	itemSummaries := make([]StackSummary, 0, len(stacks.Items))

	for i, item := range stacks.Items {
		itemSummaries[i] = StackSummary{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			ProjectName: item.Project.Name,
		}
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, itemSummaries)
	if err != nil {
		return ToolError(logger, "failed to marshal stacks", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}

// StackSummary is a restricted set of details for listing stacks
type StackSummary struct {
	ID          string `json:"ID"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ProjectName string `json:"project"`
}
