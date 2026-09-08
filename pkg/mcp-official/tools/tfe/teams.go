// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TeamSummary holds a trimmed view of a single Terraform team.
type TeamSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	UserCount  int    `json:"users-count"`
}

// TeamSummaryList contains the list of team summaries and pag details
type TeamSummaryList struct {
	Items []*TeamSummary `json:"items"`
	*tfe.Pagination
}

// TeamDetails holds the full detail view of a single Terraform team. go-tfe's
// Team type only carries jsonapi tags, so we will need to map it onto our own type to keep
// the tool output shaped like the rest of the tools.
type TeamDetails struct {
	ID                         string             `json:"id"`
	Name                       string             `json:"name"`
	Visibility                 string             `json:"visibility"`
	UserCount                  int                `json:"users-count"`
	SSOTeamID                  string             `json:"sso-team-id,omitempty"`
	IsUnified                  bool               `json:"is-unified"`
	AllowMemberTokenManagement bool               `json:"allow-member-token-management"`
	OrganizationAccess         *TeamOrgAccess     `json:"organization-access,omitempty"`
	Permissions                *TeamPermissions   `json:"permissions,omitempty"`
	Users                      []*TeamUserSummary `json:"users,omitempty"`
	SCIMLinked                 *bool              `json:"scim-linked,omitempty"`
	SCIMSyncPaused             *bool              `json:"scim-sync-paused,omitempty"`
	SCIMGroupName              *string            `json:"scim-group-name,omitempty"`
	SCIMUpdatedAt              *time.Time         `json:"scim-updated-at,omitempty"`
}

// TeamOrgAccess holds the organization level permissions granted to a team.
type TeamOrgAccess struct {
	ManagePolicies           bool `json:"manage-policies"`
	ManagePolicyOverrides    bool `json:"manage-policy-overrides"`
	DelegatePolicyOverrides  bool `json:"delegate-policy-overrides"`
	ManageWorkspaces         bool `json:"manage-workspaces"`
	ManageVCSSettings        bool `json:"manage-vcs-settings"`
	ManageProviders          bool `json:"manage-providers"`
	ManageModules            bool `json:"manage-modules"`
	ManageRunTasks           bool `json:"manage-run-tasks"`
	ManageProjects           bool `json:"manage-projects"`
	ReadWorkspaces           bool `json:"read-workspaces"`
	ReadProjects             bool `json:"read-projects"`
	ManageMembership         bool `json:"manage-membership"`
	ManageTeams              bool `json:"manage-teams"`
	ManageOrganizationAccess bool `json:"manage-organization-access"`
	AccessSecretTeams        bool `json:"access-secret-teams"`
	ManageAgentPools         bool `json:"manage-agent-pools"`
}

// TeamPermissions holds what the current token is allowed to do to this team.
type TeamPermissions struct {
	CanDestroy          bool `json:"can-destroy"`
	CanUpdateMembership bool `json:"can-update-membership"`
}

// TeamUserSummary is a trimmed view of a team member. Teams.Read takes
// no include options, so only the user ID comes back on the relation.
type TeamUserSummary struct {
	ID string `json:"id"`
}

// ListTeamsArguments holds the input parameters for listing teams within an organization.
type ListTeamsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`

	// Optional fields
	TeamNames   string `json:"team_names,omitempty" jsonschema:"Comma-separated list of exact team names to filter by. Only teams whose name exactly matches one of the provided values are returned. Example: owners,developers,platform-infra"`
	SearchQuery string `json:"search_query,omitempty" jsonschema:"Substring search query to filter teams by name. Returns all teams whose name contains the query string. Example: platform"`
	Page        int    `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize    int    `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

func ListTeamsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_teams",
		Description: "List teams within a Terraform Cloud organization. Returns a summary of each team including ID, name, visibility, and member count. Optionally filter by exact team names or a search query. Supports pagination for large result sets.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List teams in a Terraform Cloud organization",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListTeamsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListTeamsArguments) (*mcp.CallToolResult, *TeamSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	searchQuery := strings.TrimSpace(input.SearchQuery)

	var teamNames []string
	if names := strings.TrimSpace(input.TeamNames); names != "" {
		teamNames = strings.Split(names, ",")
		for i, n := range teamNames {
			teamNames[i] = strings.TrimSpace(n)
		}
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	teams, err := tfeClient.Teams.List(ctx, terraformOrgName, &tfe.TeamListOptions{
		Names: teamNames,
		Query: searchQuery,
		ListOptions: tfe.ListOptions{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list teams in org '%s': %w", terraformOrgName, err)
	}
	if len(teams.Items) == 0 {
		return nil, nil, fmt.Errorf("no teams to list in organization %q", terraformOrgName)
	}

	summaries := make([]*TeamSummary, len(teams.Items))
	for i, t := range teams.Items {
		summaries[i] = &TeamSummary{
			ID:         t.ID,
			Name:       t.Name,
			Visibility: t.Visibility,
			UserCount:  t.UserCount,
		}
	}
	return nil, &TeamSummaryList{
		Items:      summaries,
		Pagination: teams.Pagination,
	}, nil
}

// GetTeamArguments holds the input params for fetching a single team.
type GetTeamArguments struct {
	// Required field
	TeamID string `json:"team_id" jsonschema:"The ID of the team to retrieve (e.g. team-abc123)"`
}

func GetTeamTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_team",
		Description: "Fetch full details for a single team by ID, including member IDs, organization access permissions, and SSO settings.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Fetch full details for a single team by ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetTeamFunc(ctx context.Context, request *mcp.CallToolRequest, input GetTeamArguments) (*mcp.CallToolResult, *TeamDetails, error) {
	teamID := strings.TrimSpace(input.TeamID)

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	team, err := tfeClient.Teams.Read(ctx, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read team %q: %w", teamID, err)
	}
	return nil, teamToDetails(team), nil
}

// teamToDetails maps go-tfe's Team onto TeamDetails. this is kept separate from
// GetTeamFunc so the mapping can be tested without a TFE client.
func teamToDetails(team *tfe.Team) *TeamDetails {
	if team == nil {
		return nil
	}

	details := &TeamDetails{
		ID:                         team.ID,
		Name:                       team.Name,
		Visibility:                 team.Visibility,
		UserCount:                  team.UserCount,
		SSOTeamID:                  team.SSOTeamID,
		IsUnified:                  team.IsUnified,
		AllowMemberTokenManagement: team.AllowMemberTokenManagement,
		SCIMLinked:                 team.SCIMLinked,
		SCIMSyncPaused:             team.SCIMSyncPaused,
		SCIMGroupName:              team.SCIMGroupName,
		SCIMUpdatedAt:              team.SCIMUpdatedAt,
	}

	if access := team.OrganizationAccess; access != nil {
		details.OrganizationAccess = &TeamOrgAccess{
			ManagePolicies:           access.ManagePolicies,
			ManagePolicyOverrides:    access.ManagePolicyOverrides,
			DelegatePolicyOverrides:  access.DelegatePolicyOverrides,
			ManageWorkspaces:         access.ManageWorkspaces,
			ManageVCSSettings:        access.ManageVCSSettings,
			ManageProviders:          access.ManageProviders,
			ManageModules:            access.ManageModules,
			ManageRunTasks:           access.ManageRunTasks,
			ManageProjects:           access.ManageProjects,
			ReadWorkspaces:           access.ReadWorkspaces,
			ReadProjects:             access.ReadProjects,
			ManageMembership:         access.ManageMembership,
			ManageTeams:              access.ManageTeams,
			ManageOrganizationAccess: access.ManageOrganizationAccess,
			AccessSecretTeams:        access.AccessSecretTeams,
			ManageAgentPools:         access.ManageAgentPools,
		}
	}

	if permissions := team.Permissions; permissions != nil {
		details.Permissions = &TeamPermissions{
			CanDestroy:          permissions.CanDestroy,
			CanUpdateMembership: permissions.CanUpdateMembership,
		}
	}

	for _, user := range team.Users {
		if user == nil {
			continue
		}
		details.Users = append(details.Users, &TeamUserSummary{ID: user.ID})
	}

	return details
}
