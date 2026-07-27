// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListTeams creates a tool to get all the Teams for a given Terraform Organization.
func ListTeams(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_teams",
			mcp.WithDescription(`List teams within a Terraform Cloud organization. Returns a summary of each team including ID, name, visibility, and member count. Optionally filter by exact team names or a search query, and sideload related users or organization memberships. Supports pagination for large result sets.`),
			mcp.WithTitleAnnotation("List teams in a Terraform Cloud organization."),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			utils.WithPagination(),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription),
			),
			mcp.WithString("include_relations",
				mcp.Description(`Comma-separated list of related resources to embed in each team result. Valid values: "users", "organization-memberships". Already set default to: "users,organization-memberships"`),
			),
			mcp.WithString("team_names",
				mcp.Description(`Comma-separated list of exact team names to filter by. Only teams whose name exactly matches one of the provided values are returned. Example: "owners,developers,platform-infra"`),
			),
			mcp.WithString("search_query",
				mcp.Description(`Substring search query to filter teams by name. Returns all teams whose name contains the query string. Example: "platform"`),
			),
		),

		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listTeamsHandler(ctx, request, logger)
		},
	}
}

// listTeamsHandler handles tool logics and functionality
func listTeamsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {

	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "Missing required input: terraform_org_name", err)
	}
	includeRelation := request.GetString("include_relations", "users, organization-memberships")
	teamName := request.GetString("team_names", "")
	searchQuery := request.GetString("search_query", "")

	terraformOrgName = strings.TrimSpace(terraformOrgName)
	includeRelation = strings.TrimSpace(includeRelation)
	teamName = strings.TrimSpace(teamName)
	searchQuery = strings.TrimSpace(searchQuery)

	var teamNames []string
	if teamName != "" {
		teamNames = strings.Split(teamName, ",")
		for i, n := range teamNames {
			teamNames[i] = strings.TrimSpace(n)
		}
	}
	var includeRelations []tfe.TeamIncludeOpt
	if includeRelation != "" {
		parts := strings.Split(includeRelation, ",")
		for _, p := range parts {
			includeRelations = append(includeRelations, tfe.TeamIncludeOpt(strings.TrimSpace(p)))
		}
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return ToolError(logger, "Invalid pagination parameters", err)
	}

	teams, err := tfeClient.Teams.List(ctx, terraformOrgName, &tfe.TeamListOptions{
		Include: includeRelations,
		Names:   teamNames,
		Query:   searchQuery,

		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})
	if err != nil {
		return ToolErrorf(logger, "Failed to list teams in org %q", terraformOrgName)
	}
	if len(teams.Items) == 0 {
		return ToolErrorf(logger, "No teams to list in organization %q", terraformOrgName)
	}

	teamSummaries := make([]*TeamsSummary, len(teams.Items))
	for i, t := range teams.Items {
		teamSummaries[i] = &TeamsSummary{
			ID:                      t.ID,
			Name:                    t.Name,
			Visibility:              t.Visibility,
			UserCount:               t.UserCount,
			Users:                   t.Users,
			OrganizationMemberships: t.OrganizationMemberships,
		}
	}

	teamsJSON, err := json.Marshal(&TeamsSummaryList{
		Items:      teamSummaries,
		Pagination: teams.Pagination,
	})
	if err != nil {
		return ToolError(logger, "Failed to marshal teams", err)
	}

	return mcp.NewToolResultText(string(teamsJSON)), nil

}

// TeamsSummary is a truncated summary of Teams details for listing
type TeamsSummary struct {
	ID                      string                        `json:"id"`
	Name                    string                        `json:"name"`
	Visibility              string                        `json:"visibility"`
	UserCount               int                           `json:"users-count"`
	Users                   []*tfe.User                   `json:"users,omitempty"`
	OrganizationMemberships []*tfe.OrganizationMembership `json:"organization-memberships,omitempty"`
}

// TeamsSummaryList is a list of Team summaries with pagination
type TeamsSummaryList struct {
	Items []*TeamsSummary `json:"items"`
	*tfe.Pagination
}
