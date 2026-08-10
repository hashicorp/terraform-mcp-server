// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetTeam(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GetTeam(logger)

		assert.Equal(t, "get_team", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Fetch full details for a single team")
		assert.NotNil(t, tool.Handler)

		// Verify annotations: read-only, not destructive, calls external TFE API
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
		assert.True(t, *tool.Tool.Annotations.OpenWorldHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_id")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name          string
			params        map[string]interface{}
			expectTeamErr bool
		}{
			{
				name: "team_id present",
				params: map[string]interface{}{
					"team_id": "team-abc123xyz",
				},
				expectTeamErr: false,
			},
			{
				name:          "missing team_id",
				params:        map[string]interface{}{},
				expectTeamErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				teamID, teamErr := request.RequireString("team_id")

				if tt.expectTeamErr {
					assert.Error(t, teamErr)
					assert.Contains(t, teamErr.Error(), "team_id")
				} else {
					assert.NoError(t, teamErr)
					assert.Equal(t, tt.params["team_id"], teamID)
				}
			})
		}
	})
}