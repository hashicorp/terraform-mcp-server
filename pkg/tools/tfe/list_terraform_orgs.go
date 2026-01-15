// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListTerraformOrgs creates a tool to get terraform organizations.
func ListTerraformOrgs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_terraform_orgs",
			mcp.WithDescription(`Fetches a list of all Terraform organizations. Supports Pagination for large result sets.`),
			mcp.WithTitleAnnotation("List all Terraform organizations"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listTerraformOrgsHandler(ctx, req, logger)
		},
	}
}

func listTerraformOrgsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "invalid pagination parameters", err)
	}

	orgs, err := tfeClient.Organizations.List(ctx, &tfe.OrganizationListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})
	if err != nil {
		return ToolError(logger, "failed to list Terraform organizations", err)
	}
	if len(orgs.Items) == 0 {
		return ToolError(logger, "no organizations to list", err)
	}

	orgSummaries := make([]*OrganizationSummary, len(orgs.Items))
	for i, o := range orgs.Items {
		orgSummaries[i] = &OrganizationSummary{
			Name:      o.Name,
			Email:     o.Email,
			CreatedAt: o.CreatedAt,
		}
	}

	orgsJSON, err := json.Marshal(&OrganizationSummaryList{
		Items:      orgSummaries,
		Pagination: orgs.Pagination,
	})
	if err != nil {
		return ToolError(logger, "failed to marshal organization names", err)
	}

	return mcp.NewToolResultText(string(orgsJSON)), nil
}

// OrganizationSummary is a truncated summary of organization details for listing
type OrganizationSummary struct {
	Name      string    `json:"organization_name"`
	Email     string    `json:"organization_email"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationSummaryList is a list of organization summaries with pagination
type OrganizationSummaryList struct {
	Items []*OrganizationSummary `json:"items"`
	*tfe.Pagination
}
