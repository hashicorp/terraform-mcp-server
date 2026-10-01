// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TeamDetails is what get_team returns. tfe.Team only has jsonapi tags so
// we map it. Teams.Read has no include options, so the users relation only
// comes back with IDs. field names match tfe.Team so the drift test can
// line them up.
type TeamDetails struct {
	ID                         string            `json:"id"`
	Name                       string            `json:"name"`
	Visibility                 string            `json:"visibility"`
	UserCount                  int               `json:"users-count"`
	SSOTeamID                  string            `json:"sso-team-id,omitempty"`
	IsUnified                  bool              `json:"is-unified"`
	AllowMemberTokenManagement bool              `json:"allow-member-token-management"`
	OrganizationAccess         *TeamOrgAccess    `json:"organization-access,omitempty"`
	Permissions                *TeamPermissions  `json:"permissions,omitempty"`
	Users                      []TeamUserSummary `json:"users"`
	SCIMLinked                 *bool             `json:"scim-linked,omitempty"`
	SCIMSyncPaused             *bool             `json:"scim-sync-paused,omitempty"`
	SCIMGroupName              *string           `json:"scim-group-name,omitempty"`
	SCIMUpdatedAt              *time.Time        `json:"scim-updated-at,omitempty"`
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

type TeamUserSummary struct {
	ID string `json:"id"`
}

type GetTeamArguments struct {
	// Required field
	TeamID string `json:"team_id" jsonschema:"The ID of the team to fetch (e.g., 'team-abc123def456')"`
}

func GetTeamTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_team",
		Description: `Fetches detailed information about a Terraform team by its ID, including member IDs, organization access permissions, and SSO settings. If the team ID isn't already known, call "list_teams" first.`,
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"id":                            {Type: "string"},
				"name":                          {Type: "string"},
				"visibility":                    {Type: "string"},
				"users-count":                   {Type: "integer"},
				"sso-team-id":                   {Type: "string"},
				"is-unified":                    {Type: "boolean"},
				"allow-member-token-management": {Type: "boolean"},
				"organization-access": {
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"manage-policies":            {Type: "boolean"},
						"manage-policy-overrides":    {Type: "boolean"},
						"delegate-policy-overrides":  {Type: "boolean"},
						"manage-workspaces":          {Type: "boolean"},
						"manage-vcs-settings":        {Type: "boolean"},
						"manage-providers":           {Type: "boolean"},
						"manage-modules":             {Type: "boolean"},
						"manage-run-tasks":           {Type: "boolean"},
						"manage-projects":            {Type: "boolean"},
						"read-workspaces":            {Type: "boolean"},
						"read-projects":              {Type: "boolean"},
						"manage-membership":          {Type: "boolean"},
						"manage-teams":               {Type: "boolean"},
						"manage-organization-access": {Type: "boolean"},
						"access-secret-teams":        {Type: "boolean"},
						"manage-agent-pools":         {Type: "boolean"},
					},
					AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
				},
				"permissions": {
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"can-destroy":           {Type: "boolean"},
						"can-update-membership": {Type: "boolean"},
					},
					AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
				},
				"users": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"id": {Type: "string"},
						},
						Required:             []string{"id"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
				"scim-linked":      {Type: "boolean"},
				"scim-sync-paused": {Type: "boolean"},
				"scim-group-name":  {Type: "string"},
				"scim-updated-at":  {Type: "string", Format: "date-time"},
			},
			Required:             []string{"id", "name", "visibility", "users-count", "is-unified", "allow-member-token-management", "users"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get a Terraform team by ID",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetTeamFunc(ctx context.Context, request *mcp.CallToolRequest, input GetTeamArguments) (*mcp.CallToolResult, *TeamDetails, error) {
	teamID := strings.TrimSpace(input.TeamID)
	if teamID == "" {
		return nil, nil, fmt.Errorf("team_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	team, err := tfeClient.Teams.Read(ctx, teamID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading team %q: %w", teamID, err)
	}

	return nil, teamToDetails(team), nil
}

// teamToDetails maps go-tfe's Team onto TeamDetails.
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
		Users:                      make([]TeamUserSummary, 0, len(team.Users)),
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
		details.Users = append(details.Users, TeamUserSummary{ID: user.ID})
	}

	return details
}
