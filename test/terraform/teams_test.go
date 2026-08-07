package terraform

import (
	"testing"

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
