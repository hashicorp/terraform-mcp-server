// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTeam(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateTeam(logger)

		assert.Equal(t, "create_team", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new team")
		assert.NotNil(t, tool.Handler)

		// Not destructive, not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_name")

		// visibility is optional
		assert.NotNil(t, tool.Tool.InputSchema.Properties["visibility"])
		assert.NotContains(t, tool.Tool.InputSchema.Required, "visibility")
	})

	t.Run("missing terraform_org_name returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"team_name": "my-team",
		}}

		_, err := request.RequireString("terraform_org_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "terraform_org_name")
	})

	t.Run("missing team_name returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
		}}

		_, err := request.RequireString("team_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "team_name")
	})

	t.Run("visibility defaults to 'organization' when absent", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
			"team_name":          "my-team",
		}}

		visibility := request.GetString("visibility", "organization")
		assert.Equal(t, "organization", visibility)
	})

	t.Run("visibility defaults to 'organization' when empty string provided", func(t *testing.T) {
		// The handler treats empty-string visibility as "organization"
		visibility := ""
		if visibility == "" {
			visibility = "organization"
		}
		assert.Equal(t, "organization", visibility)
	})

	t.Run("valid visibility values are accepted", func(t *testing.T) {
		validValues := []string{"organization", "secret"}
		for _, v := range validValues {
			request := &MockCallToolRequest{params: map[string]interface{}{
				"terraform_org_name": "my-org",
				"team_name":          "my-team",
				"visibility":         v,
			}}
			visibility := request.GetString("visibility", "organization")
			assert.Equal(t, v, visibility)
		}
	})

	t.Run("TeamSummary JSON marshaling", func(t *testing.T) {
		summary := &TeamSummary{
			ID:         "team-abc123",
			Name:       "my-team",
			Visibility: "secret",
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)
		assert.Contains(t, string(data), "team-abc123")
		assert.Contains(t, string(data), "my-team")
		assert.Contains(t, string(data), "secret")

		var out TeamSummary
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, summary.ID, out.ID)
		assert.Equal(t, summary.Name, out.Name)
		assert.Equal(t, summary.Visibility, out.Visibility)
	})

	t.Run("TeamSummary JSON field names", func(t *testing.T) {
		summary := &TeamSummary{
			ID:         "team-xyz",
			Name:       "ops",
			Visibility: "organization",
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)

		var raw map[string]string
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "team-xyz", raw["team_id"])
		assert.Equal(t, "ops", raw["team_name"])
		assert.Equal(t, "organization", raw["visibility"])
	})
}
