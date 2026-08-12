package terraform

import (
	"slices"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListTeams(t *testing.T) {
	client := tfeClient(t)
	requireTeamsEntitlement(t, client)

	t.Run("list all teams", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "list_teams", map[string]any{
			"terraform_org_name": tfeOrgName,
		})

		require.False(t, result.IsError, "Tool call result should not be an error")
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Tool call result should not contain an empty list")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.id").String(), "Tool call result should contain team IDs")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.name").String(), "Tool call result should contain team names")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.visibility").String(), "Tool call result should contain team visibility")
		assert.True(t, gjson.Get(resultText, "items.0.users-count").Exists(), "Tool call result should contain user count field")
	})

	t.Run("filter by team name", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "list_teams", map[string]any{
			"terraform_org_name": tfeOrgName,
			"team_names":         "owners",
		})

		require.False(t, result.IsError, "Tool call result should not be an error")
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.Equal(t, gjson.Get(resultText, "items.0.name").String(), "owners", "Filtered result should only contain the 'owners' team")
	})

	t.Run("filter by search query", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "list_teams", map[string]any{
			"terraform_org_name": tfeOrgName,
			"search_query":       "owners",
		})

		require.False(t, result.IsError, "Tool call result should not be an error")
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Search query should return at least one matching team")
	})
}

func TestCreateTeam(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	requireTeamsEntitlement(t, client)
	teamName := randomName("team-")
	visibility := "organization"

	result, resultText := callTool(t, s, "create_team", map[string]any{
		"terraform_org_name": tfeOrgName,
		"team_name":          teamName,
		"visibility":         visibility,
	})

	require.False(t, result.IsError, "create_team returned an error: %s", resultText)
	require.NotEmpty(t, resultText, "Tool call result must not be empty")

	teamID := gjson.Get(resultText, "team_id").String()
	require.NotEmpty(t, teamID, "Tool response should include a team ID")
	defer client.Teams.Delete(t.Context(), teamID)

	assert.Equal(t, teamName, gjson.Get(resultText, "team_name").String(),
		"Tool response should report the created team name")

	assert.Equal(t, visibility, gjson.Get(resultText, "visibility").String(),
		"Tool response should report the requested visibility")

	// Verify against the TFE API directly
	createdTeam, err := client.Teams.Read(t.Context(), teamID)
	require.NoError(t, err, "Team reported as created but produced an error when reading")
	assert.Equal(t, teamName, createdTeam.Name, "Created team name does not match requested name")
	assert.Equal(t, visibility, createdTeam.Visibility)
	assert.Zero(t, createdTeam.UserCount)
}

func TestGetTeam(t *testing.T) {
	client := tfeClient(t)
	requireTeamsEntitlement(t, client)

	// Get a real team ID to look up
	teams, err := client.Teams.List(t.Context(), tfeOrgName, nil)
	require.NoError(t, err)
	require.NotEmpty(t, teams.Items, "Expected at least one team in the organization")
	teamID := teams.Items[0].ID

	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "get_team", map[string]any{
		"team_id": teamID,
	})

	require.False(t, result.IsError, "Tool call result should not be an error")
	require.NotEmpty(t, resultText, "Tool call result must not be empty")

	// MarshalPayloadWithoutIncluded returns a JSON:API envelope, not a flat object.
	assert.Equal(t, teamID, gjson.Get(resultText, "data.id").String(), "Response should contain the requested team ID")
	assert.NotEmpty(t, gjson.Get(resultText, "data.attributes.name").String(), "Response should contain the team name")
	assert.NotEmpty(t, gjson.Get(resultText, "data.attributes.visibility").String(), "Response should contain the team visibility")
	assert.True(t, gjson.Get(resultText, "data.attributes.users-count").Exists(), "Response should contain the user count field")
}

func TestAddTeamMember(t *testing.T) {
	client := tfeClient(t)
	requireTeamsEntitlement(t, client)

	team, err := client.Teams.Create(t.Context(), tfeOrgName, tfe.TeamCreateOptions{
		Name: tfe.String(randomName("team-")),
	})
	require.NoError(t, err, "Failed to create test team")
	defer client.Teams.Delete(t.Context(), team.ID)

	// Look up users's organization membership ID so we can test both paths.
	memberships, err := client.OrganizationMemberships.List(t.Context(), tfeOrgName, &tfe.OrganizationMembershipListOptions{
		Emails: []string{tfeUserEmail},
	})
	require.NoError(t, err, "Failed to list organization memberships")
	require.NotEmpty(t, memberships.Items, "Expected %v to be a member of the organization", tfeUsername)
	orgMembershipID := memberships.Items[0].ID

	t.Run("add member by username", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "add_team_member", map[string]any{
			"team_id":  team.ID,
			"username": tfeUsername,
		})

		require.False(t, result.IsError, "add_team_member returned an error: %s", resultText)
		assert.Contains(t, resultText, team.ID, "Success message should reference the team ID")

		// Verify via the TFE API directly.
		members, err := client.TeamMembers.List(t.Context(), team.ID)
		require.NoError(t, err, "Failed to list team members after add")
		found := slices.ContainsFunc(members, func(m *tfe.User) bool {
			return m.Username == tfeUsername
		})
		assert.True(t, found, tfeUsername+" should be a member of the team after add_team_member")

		// Remove the member so the membership-ID sub-test starts from a clean state.
		_ = client.TeamMembers.Remove(t.Context(), team.ID, tfe.TeamMemberRemoveOptions{
			Usernames: []string{tfeUsername},
		})
	})

	t.Run("add member by organization membership ID", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "add_team_member", map[string]any{
			"team_id":                    team.ID,
			"organization_membership_id": orgMembershipID,
		})

		require.False(t, result.IsError, "add_team_member returned an error: %s", resultText)
		assert.Contains(t, resultText, team.ID, "Success message should reference the team ID")

		// Verify via the TFE API directly.
		members, err := client.TeamMembers.List(t.Context(), team.ID)
		require.NoError(t, err, "Failed to list team members after add")
		found := slices.ContainsFunc(members, func(m *tfe.User) bool {
			return m.Username == tfeUsername
		})
		assert.True(t, found, tfeUsername+" should be a member of the team after add by membership ID")
	})

	t.Run("errors when neither username nor membership ID is provided", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "add_team_member", map[string]any{
			"team_id": team.ID,
		})

		require.True(t, result.IsError, "Tool call should return an error when no member identifier is provided")
		assert.Contains(t, resultText, "username", "Error message should mention the missing inputs")
	})

	t.Run("errors when both username and membership ID are provided", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "add_team_member", map[string]any{
			"team_id":                    team.ID,
			"username":                   tfeUsername,
			"organization_membership_id": orgMembershipID,
		})

		require.True(t, result.IsError, "Tool call should return an error when both identifiers are provided")
		assert.Contains(t, resultText, "username", "Error message should mention the conflicting inputs")
	})
}

func TestGrantTeamAccess(t *testing.T) {
	client := tfeClient(t)
	requireTeamsEntitlement(t, client)

	// Create a temporary team for all sub-tests so we never touch the owners group.
	team, err := client.Teams.Create(t.Context(), tfeOrgName, tfe.TeamCreateOptions{
		Name: tfe.String(randomName("team-")),
	})
	require.NoError(t, err, "Failed to create test team")

	// Safety net: if the delete tool fails mid-test, clean up via the API.
	defer client.Teams.Delete(t.Context(), team.ID)
	teamID := team.ID

	t.Run("grant workspace access", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		// Create a temporary workspace to grant access to.
		ws, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
			Name: tfe.String(randomName("ws-")),
		})
		require.NoError(t, err, "Failed to create test workspace")
		defer client.Workspaces.SafeDeleteByID(t.Context(), ws.ID)

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"workspace_id": ws.ID,
			"access_level": "read",
		})

		require.False(t, result.IsError, "grant_team_access returned an error: %s", resultText)
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.NotEmpty(t, gjson.Get(resultText, "id").String(), "Response should contain a grant ID")
		assert.Equal(t, teamID, gjson.Get(resultText, "team_id").String(), "Response should contain the requested team ID")
		assert.Equal(t, ws.ID, gjson.Get(resultText, "workspace_id").String(), "Response should contain the requested workspace ID")
		assert.Equal(t, "read", gjson.Get(resultText, "access").String(), "Response should reflect the requested access level")
	})

	t.Run("grant project access", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		// Create a temporary project to grant access to.
		project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
			Name: randomName("proj-"),
		})
		require.NoError(t, err, "Failed to create test project")
		defer client.Projects.Delete(t.Context(), project.ID)

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"project_id":   project.ID,
			"access_level": "read",
		})

		require.False(t, result.IsError, "grant_team_access returned an error: %s", resultText)
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.NotEmpty(t, gjson.Get(resultText, "id").String(), "Response should contain a grant ID")
		assert.Equal(t, teamID, gjson.Get(resultText, "team_id").String(), "Response should contain the requested team ID")
		assert.Equal(t, project.ID, gjson.Get(resultText, "project_id").String(), "Response should contain the requested project ID")
		assert.Equal(t, "read", gjson.Get(resultText, "access").String(), "Response should reflect the requested access level")
	})

	t.Run("error on both workspace_id and project_id", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"workspace_id": "ws-fakeid",
			"project_id":   "prj-fakeid",
			"access_level": "read",
		})

		require.True(t, result.IsError, "Expected an error when both workspace_id and project_id are provided")
		assert.Contains(t, resultText, "Only one of workspace_id or project_id may be provided")
	})

	t.Run("error on neither workspace_id nor project_id", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"access_level": "read",
		})

		require.True(t, result.IsError, "Expected an error when neither workspace_id nor project_id is provided")
		assert.Contains(t, resultText, "One of workspace_id or project_id must be provided")
	})

	t.Run("error on invalid workspace access level", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"workspace_id": "ws-fakeid",
			"access_level": "maintain",
		})

		require.True(t, result.IsError, "Expected an error when 'maintain' is used for workspace access")
		assert.Contains(t, resultText, "Invalid Team access level")
	})

	t.Run("error on invalid project access level", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "grant_team_access", map[string]any{
			"team_id":      teamID,
			"project_id":   "prj-fakeid",
			"access_level": "plan",
		})

		require.True(t, result.IsError, "Expected an error when 'plan' is used for project access")
		assert.Contains(t, resultText, "Invalid Team Project access level")
	})
}

func TestDeleteTeam(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	requireTeamsEntitlement(t, client)

	s := newTestingSession(t)
	defer s.Close()

	// Create a team directly via the TFE API — independent of the tool under test.
	team, err := client.Teams.Create(t.Context(), tfeOrgName, tfe.TeamCreateOptions{
		Name: tfe.String(randomName("team-")),
	})
	require.NoError(t, err, "setup: failed to create team via TFE API")

	// Safety net: if the delete tool fails mid-test, clean up via the API.
	defer client.Teams.Delete(t.Context(), team.ID)

	t.Run("deletes an existing team", func(t *testing.T) {
		result, resultText := callTool(t, s, "delete_team", map[string]any{
			"team_id": team.ID,
		})

		require.False(t, result.IsError, "delete_team should not return an error for an existing team: %s", resultText)
		assert.Contains(t, resultText, team.ID, "response should reference the deleted team_id")

		// Confirm the team is gone via the API directly.
		_, err := client.Teams.Read(t.Context(), team.ID)
		assert.Error(t, err, "team should no longer exist in the TFE API after deletion")
	})

	t.Run("returns an error for a non-existent team_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "delete_team", map[string]any{
			"team_id": "team-doesnotexist000",
		})

		require.True(t, result.IsError, "delete_team should return an error for a non-existent team_id")
		assert.Contains(t, resultText, "team-doesnotexist000", "error message should reference the unknown team_id")
	})
}
