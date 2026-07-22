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

// ListStateVersions creates a tool to get all the state versions for a given workspace.
func ListStateVersions(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"list_state_versions",
			mcp.WithDescription("List all the State Versions for a given workspace and org name."),
			mcp.WithTitleAnnotation(`List all States Versions`),
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
			return listStateVersionsHandler(ctx, req, logger)
		},
	}
}

// listStateVersionsHandler handles tool logics and functionality
func listStateVersionsHandler(
	ctx context.Context,
	request mcp.CallToolRequest,
	logger *log.Logger) (*mcp.CallToolResult, error) {

	// Init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Required params
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "Missing required input: terraform_org_name", err)
	}
	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "Missing required input: workspace_name", err)
	}
	// Clean params
	terraformOrgName = strings.TrimSpace(terraformOrgName)
	workspaceName = strings.TrimSpace(workspaceName)

	// Optional pagination params
	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "Invalid pagination parameters", err)
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
	if err != nil {
		return ToolError(logger, "Failed to list workspace state versions", err)
	}
	if len(sv.Items) == 0 {
		return ToolError(logger, "Workspace has no StateVersions to list", err)
	}

	// Format Output
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
	if err != nil {
		return ToolError(logger, "Failed to marshal organization names", err)
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
