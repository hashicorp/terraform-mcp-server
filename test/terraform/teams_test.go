package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListTeams(t *testing.T) {

	t.Run("list all teams", func(t *testing.T) {
		s := newTestingSession(t)
		defer s.Close()

		result, resultText := callTool(t, s, "list_teams", map[string]any{
			"terraform_org_name": "terraform-ai-ecosystem-testing",
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
			"terraform_org_name": "terraform-ai-ecosystem-testing",
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
			"terraform_org_name": "terraform-ai-ecosystem-testing",
			"search_query":       "owners",
		})

		require.False(t, result.IsError, "Tool call result should not be an error")
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Search query should return at least one matching team")
	})
}
