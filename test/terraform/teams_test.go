package terraform

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListTeams(t *testing.T) {

	// Guard: skip if the organization does not support team management.
	client := tfeClient(t)
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}

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
	teamName := randomName("team-")
	visibility := "organization"

	// Guard: skip if the organization does not support team management.
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)

	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}

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
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}

	// Get a real team ID to look up (the owners team always exists)
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

	// Guard: skip if the organization does not support team management.
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}

	team, err := client.Teams.Create(t.Context(), tfeOrgName, tfe.TeamCreateOptions{
		Name: tfe.String(randomName("team-")),
	})
	require.NoError(t, err, "Failed to create test team")
	defer client.Teams.Delete(t.Context(), team.ID)

	// Look up doormat-at-hashicorp_com's organization membership ID so we can test both paths.
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
		var found bool
		for _, m := range members {
			if m.Username == tfeUsername {
				found = true
				break
			}
		}
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
		var found bool
		for _, m := range members {
			if m.Username == tfeUsername {
				found = true
				break
			}
		}
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
