// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTeamTool(t *testing.T) {
	tool := GetTeamTool()

	assert.Equal(t, "get_team", tool.Name)
	assert.Contains(t, tool.Description, "Fetches detailed information about a Terraform team")
	assert.Nil(t, tool.InputSchema, "get_team relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get a Terraform team by ID", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetTeamArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetTeamArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"team_id"}, schema.Required)

	teamID := schema.Properties["team_id"]
	require.NotNil(t, teamID)
	assert.Equal(t, "string", teamID.Type)
}

func TestGetTeamFunc_RequiresTeamID(t *testing.T) {
	tests := []struct {
		name   string
		teamID string
	}{
		{name: "empty", teamID: ""},
		{name: "whitespace only", teamID: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetTeamFunc(t.Context(), nil, GetTeamArguments{TeamID: tt.teamID})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "team_id must not be blank")
		})
	}
}

// These fail when go-tfe adds an attribute the details structs don't carry.
func TestTeamDetailsCoversTeamAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.Team{}), reflect.TypeOf(TeamDetails{}), nil)
}

func TestTeamOrgAccessCoversAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.OrganizationAccess{}), reflect.TypeOf(TeamOrgAccess{}), nil)
}

func TestTeamPermissionsCoversAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.TeamPermissions{}), reflect.TypeOf(TeamPermissions{}), nil)
}

func TestTeamToDetails(t *testing.T) {
	t.Run("nil team", func(t *testing.T) {
		assert.Nil(t, teamToDetails(nil))
	})

	t.Run("maps top level fields", func(t *testing.T) {
		details := teamToDetails(&tfe.Team{
			ID:                         "team-abc123",
			Name:                       "platform-infra",
			Visibility:                 "organization",
			UserCount:                  3,
			SSOTeamID:                  "sso-team-1",
			IsUnified:                  true,
			AllowMemberTokenManagement: true,
		})

		require.NotNil(t, details)
		assert.Equal(t, "team-abc123", details.ID)
		assert.Equal(t, "platform-infra", details.Name)
		assert.Equal(t, "organization", details.Visibility)
		assert.Equal(t, 3, details.UserCount)
		assert.Equal(t, "sso-team-1", details.SSOTeamID)
		assert.True(t, details.IsUnified)
		assert.True(t, details.AllowMemberTokenManagement)
	})

	t.Run("leaves nested fields nil when absent", func(t *testing.T) {
		details := teamToDetails(&tfe.Team{ID: "team-abc123"})

		require.NotNil(t, details)
		assert.Nil(t, details.OrganizationAccess)
		assert.Nil(t, details.Permissions)
		assert.Empty(t, details.Users)
	})

	t.Run("maps organization access", func(t *testing.T) {
		details := teamToDetails(&tfe.Team{
			ID: "team-abc123",
			OrganizationAccess: &tfe.OrganizationAccess{
				ManageWorkspaces: true,
				ManageTeams:      true,
				ReadProjects:     true,
			},
		})

		require.NotNil(t, details.OrganizationAccess)
		assert.True(t, details.OrganizationAccess.ManageWorkspaces)
		assert.True(t, details.OrganizationAccess.ManageTeams)
		assert.True(t, details.OrganizationAccess.ReadProjects)
		assert.False(t, details.OrganizationAccess.ManagePolicies)
	})

	t.Run("maps permissions", func(t *testing.T) {
		details := teamToDetails(&tfe.Team{
			ID: "team-abc123",
			Permissions: &tfe.TeamPermissions{
				CanDestroy:          true,
				CanUpdateMembership: false,
			},
		})

		require.NotNil(t, details.Permissions)
		assert.True(t, details.Permissions.CanDestroy)
		assert.False(t, details.Permissions.CanUpdateMembership)
	})

	t.Run("maps user ids and skips nil entries", func(t *testing.T) {
		details := teamToDetails(&tfe.Team{
			ID: "team-abc123",
			Users: []*tfe.User{
				{ID: "user-1"},
				nil,
				{ID: "user-2"},
			},
		})

		require.Len(t, details.Users, 2)
		assert.Equal(t, "user-1", details.Users[0].ID)
		assert.Equal(t, "user-2", details.Users[1].ID)
	})
}

func TestGetTeamToolOutputSchema(t *testing.T) {
	schema := inputSchema(t, GetTeamTool().OutputSchema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Required, "users")

	users := schema.Properties["users"]
	require.NotNil(t, users)
	assert.Equal(t, "array", users.Type)

	assert.Equal(t, "object", schema.Properties["organization-access"].Type)
	assert.Equal(t, "boolean", schema.Properties["scim-linked"].Type)
	assert.NotContains(t, schema.Required, "scim-linked")
}

func TestTeamToDetails_UsersNeverNil(t *testing.T) {
	details := teamToDetails(&tfe.Team{ID: "team-abc123"})
	require.NotNil(t, details.Users, "users must marshal as [] not null")
	assert.Empty(t, details.Users)
}
