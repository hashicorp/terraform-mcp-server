// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTeamsTool(t *testing.T) {
	tool := ListTeamsTool()

	assert.Equal(t, "list_teams", tool.Name)
	assert.Contains(t, tool.Description, "List teams within a Terraform Cloud organization")

	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
}

func TestGetTeamTool(t *testing.T) {
	tool := GetTeamTool()

	assert.Equal(t, "get_team", tool.Name)
	assert.Contains(t, tool.Description, "Fetch full details for a single team")

	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
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
