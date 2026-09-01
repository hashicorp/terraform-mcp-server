// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	validTeamVisibilities        = []string{"secret", "organization"}
	validTeamAccessLevels        = []string{"admin", "read", "write", "plan"}
	validTeamProjectAccessLevels = []string{"admin", "read", "write", "maintain"}
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

// TeamCreateSummary is the response summary for a newly created team.
type TeamCreateSummary struct {
	ID         string `json:"team_id"`
	Name       string `json:"team_name"`
	Visibility string `json:"visibility"`
	UserCount  int    `json:"user_count,omitempty"`
}

// CreateTeamArguments holds the input params for creating a team.
type CreateTeamArguments struct {
	// Required fields
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	TeamName         string `json:"team_name" jsonschema:"The unique name of the team to create in the Terraform organization"`

	// Optional fields
	Visibility string `json:"visibility,omitempty" jsonschema:"Team visibility. One of: secret, organization. If omitted the API defaults to secret"`
}

func CreateTeamTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_team",
		Description: "Creates a new team in a Terraform Cloud/Enterprise organization.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new team in a Terraform organization",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func CreateTeamFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateTeamArguments) (*mcp.CallToolResult, *TeamCreateSummary, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	teamName := strings.TrimSpace(input.TeamName)
	visibility := strings.ToLower(strings.TrimSpace(input.Visibility))

	if visibility != "" && !slices.Contains(validTeamVisibilities, visibility) {
		return nil, nil, fmt.Errorf("invalid visibility %q - must be one of: %s", visibility, strings.Join(validTeamVisibilities, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	options := tfe.TeamCreateOptions{
		Name: tfe.String(teamName),
	}
	if visibility != "" {
		options.Visibility = tfe.String(visibility)
	}

	team, err := tfeClient.Teams.Create(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create team %q in org %q: %w", teamName, terraformOrgName, err)
	}

	return nil, &TeamCreateSummary{
		ID:         team.ID,
		Name:       team.Name,
		Visibility: team.Visibility,
		UserCount:  team.UserCount,
	}, nil
}

// AddTeamMemberResult reports the outcome of adding a member to a team.
type AddTeamMemberResult struct {
	TeamID string `json:"team_id"`
	Added  string `json:"added"`
}

// AddTeamMemberArguments holds the input params for adding a member to a team.
// Exactly one of Username or OrganizationMembershipID must be provided.
type AddTeamMemberArguments struct {
	// Required field
	TeamID string `json:"team_id" jsonschema:"The ID of the Terraform Cloud/Enterprise team to add members to (e.g. team-abc123def456)"`

	// one of these must be provided
	Username                 string `json:"username,omitempty" jsonschema:"Username of the member to add. Only works for users who have accepted the organization invite"`
	OrganizationMembershipID string `json:"organization_membership_id,omitempty" jsonschema:"Organization membership ID of the member to add (e.g. ou-abc123). Works for both accepted and pending organization invites. Prefer this over username when the invitee has not yet accepted"`
}

func AddTeamMemberTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "add_team_member",
		Description: "Adds a single member to a Terraform Cloud/Enterprise team. Provide either a username (accepted invites only) or an organization membership ID (accepted and pending invites), not both.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Add member to a Terraform team",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func AddTeamMemberFunc(ctx context.Context, request *mcp.CallToolRequest, input AddTeamMemberArguments) (*mcp.CallToolResult, *AddTeamMemberResult, error) {
	teamID := strings.TrimSpace(input.TeamID)
	username := strings.TrimSpace(input.Username)
	orgMembershipID := strings.TrimSpace(input.OrganizationMembershipID)

	if username == "" && orgMembershipID == "" {
		return nil, nil, fmt.Errorf("one of 'username' or 'organization_membership_id' must be provided")
	}
	if username != "" && orgMembershipID != "" {
		return nil, nil, fmt.Errorf("provide only one of 'username' or 'organization_membership_id', not both")
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	options := tfe.TeamMemberAddOptions{}
	var memberID string
	if username != "" {
		options.Usernames = []string{username}
		memberID = username
	} else {
		options.OrganizationMembershipIDs = []string{orgMembershipID}
		memberID = orgMembershipID
	}

	if err := tfeClient.TeamMembers.Add(ctx, teamID, options); err != nil {
		return nil, nil, fmt.Errorf("failed to add member %q to team %q: %w", memberID, teamID, err)
	}

	return nil, &AddTeamMemberResult{
		TeamID: teamID,
		Added:  memberID,
	}, nil
}

// TeamAccessGrant is the response summary for a granted team access. Only one of
// WorkspaceID or ProjectID is set, matching whichever target was requested.
type TeamAccessGrant struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Access      string `json:"access"`
}

// GrantTeamAccessArguments holds the input params for granting team access.
type GrantTeamAccessArguments struct {
	// Required fields
	TeamID      string `json:"team_id" jsonschema:"The ID of the team to grant access. Team IDs begin with team- (e.g. team-abc123def456)"`
	AccessLevel string `json:"access_level" jsonschema:"The permission level to grant the team. For workspace access: read, plan, write, admin. For project access: read, write, maintain, admin. plan is only valid for workspaces and maintain is only valid for projects"`

	//one of these must be provided
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"The ID of the workspace to grant the team access to. Workspace IDs begin with ws- (e.g. ws-abc123def456). Mutually exclusive with project_id"`
	ProjectID   string `json:"project_id,omitempty" jsonschema:"The ID of the project to grant the team access to. Project IDs begin with prj- (e.g. prj-abc123def456). Mutually exclusive with workspace_id"`
}

func GrantTeamAccessTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "grant_team_access",
		Description: "Grants a team permission to access a workspace or a project in Terraform Cloud/Enterprise. Provide either workspace_id (for workspace-level access) or project_id (for project-level access), not both. Returns the created access grant including its ID, team ID, target resource ID, and access level.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Grant team access to a workspace or project",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func GrantTeamAccessFunc(ctx context.Context, request *mcp.CallToolRequest, input GrantTeamAccessArguments) (*mcp.CallToolResult, *TeamAccessGrant, error) {
	teamID := strings.TrimSpace(input.TeamID)
	accessLevel := strings.ToLower(strings.TrimSpace(input.AccessLevel))
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	projectID := strings.TrimSpace(input.ProjectID)

	if workspaceID == "" && projectID == "" {
		return nil, nil, fmt.Errorf("one of workspace_id or project_id must be provided")
	}
	if workspaceID != "" && projectID != "" {
		return nil, nil, fmt.Errorf("only one of workspace_id or project_id may be provided, not both")
	}
	// validate the access level up front so a bad one doesn't get as far as
	// building a TFE client.
	if workspaceID != "" && !slices.Contains(validTeamAccessLevels, accessLevel) {
		return nil, nil, fmt.Errorf("invalid team access level %q - must be one of: %s", accessLevel, strings.Join(validTeamAccessLevels, ", "))
	}
	if projectID != "" && !slices.Contains(validTeamProjectAccessLevels, accessLevel) {
		return nil, nil, fmt.Errorf("invalid team project access level %q - must be one of: %s", accessLevel, strings.Join(validTeamProjectAccessLevels, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	if workspaceID != "" {
		access, err := tfeClient.TeamAccess.Add(ctx, tfe.TeamAccessAddOptions{
			Access:    tfe.Access(tfe.AccessType(accessLevel)),
			Workspace: &tfe.Workspace{ID: workspaceID},
			Team:      &tfe.Team{ID: teamID},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to grant team access to workspace %q: %w", workspaceID, err)
		}

		return nil, &TeamAccessGrant{
			ID:          access.ID,
			TeamID:      access.Team.ID,
			WorkspaceID: access.Workspace.ID,
			Access:      string(access.Access),
		}, nil
	}

	projectAccess, err := tfeClient.TeamProjectAccess.Add(ctx, tfe.TeamProjectAccessAddOptions{
		Access:  tfe.TeamProjectAccessType(accessLevel),
		Project: &tfe.Project{ID: projectID},
		Team:    &tfe.Team{ID: teamID},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to grant team project access to project %q: %w", projectID, err)
	}

	return nil, &TeamAccessGrant{
		ID:        projectAccess.ID,
		TeamID:    projectAccess.Team.ID,
		ProjectID: projectAccess.Project.ID,
		Access:    string(projectAccess.Access),
	}, nil
}
