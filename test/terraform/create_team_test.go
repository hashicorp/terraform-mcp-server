package terraform

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCreateTeam(t *testing.T) {

	if tfeOrgName == "" {
		t.Skip("You need to supply TFE_ORG_NAME to run this test")
	}

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	teamName := fmt.Sprintf("mcp-test-team-%d", time.Now().Unix())
	visibility := "organization"

	// Skip if the organization does not support team management.
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
