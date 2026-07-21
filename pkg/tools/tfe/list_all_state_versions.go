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
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListAllStateVersions creates a tool to get all the state versions for a given workspace.
func ListAllStateVersions(logger *log.Logger) server.ServerTool {
	// Returns Server tool
	return server.ServerTool{
		// Create new tool
		Tool: mcp.NewTool(
			"list_all_state_versions",
			mcp.WithDescription("List all the state versions for a given workspace."),
			mcp.WithTitleAnnotation("List all States Versions"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			utils.WithPagination(),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The workspace name to list state versions for"),
			),
		),

		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listAllStateVersionsHandler(ctx, req, logger)
		},
	}
}

// listAllStateVersionsHandler handles tool logics and functionality
func listAllStateVersionsHandler(
	ctx context.Context,
	request mcp.CallToolRequest,
	logger *log.Logger) (*mcp.CallToolResult, error) {

	// init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)

	// Failed to get Terraform client
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// client context nill
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Required params
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	workspaceName = strings.TrimSpace(workspaceName)

	// Optional pagination params
	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "invalid pagination parameters", err)
	}

	// List state versions
	sv, err := tfeClient.StateVersions.List(ctx, &tfe.StateVersionListOptions{
		Organization: terraformOrgName,
		Workspace:    workspaceName,
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})

	// If tool fails
	if err != nil {
		return ToolError(logger, "failed to list workspace state versions", err)
	}

	// Check if no State versions at the moment
	if len(sv.Items) == 0 {
		return ToolError(logger, "no sv's to list", err)
	}

	// Format Output as JSON
	svSummaries := make([]*StateVersionsSummary, len(sv.Items))
	for i, o := range sv.Items {
		svSummaries[i] = &StateVersionsSummary{
			ID:               o.ID,
			CreatedAt:        o.CreatedAt,
			Serial:           o.Serial,
			TerraformVersion: o.TerraformVersion,
			VCSCommitSHA:     o.VCSCommitSHA,
			VCSCommitURL:     o.VCSCommitURL,
			StateVersion:     o.StateVersion,
		}
	}

	// Marshal JSON
	svJSON, err := json.Marshal(&StateVersionsSummaryList{
		Items:      svSummaries,
		Pagination: sv.Pagination,
	})

	// Failed to marshal state versions names
	if err != nil {
		return ToolError(logger, "failed to marshal organization names", err)
	}

	return mcp.NewToolResultText(string(svJSON)), nil

}

// StateVersionsSummary is a truncated summary of State Version details for listing
type StateVersionsSummary struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	Serial           int64     `json:"serial"`
	TerraformVersion string    `json:"terraform_version"`
	VCSCommitSHA     string    `json:"vcs_commit_sha"`
	VCSCommitURL     string    `json:"vcs_commit_url"`
	StateVersion     int       `json:"state_version"`
}

// StateVersionsSummaryList is a list of state version summaries with pagination
type StateVersionsSummaryList struct {
	Items []*StateVersionsSummary `json:"items"`
	*tfe.Pagination
}
