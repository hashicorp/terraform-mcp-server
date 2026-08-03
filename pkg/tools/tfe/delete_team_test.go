// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDeleteTeam(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := DeleteTeam(logger)

		assert.Equal(t, "delete_team", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Permanently deletes a Terraform team")
		assert.NotNil(t, tool.Handler)

		// Verify it's marked as destructive
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_id")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
			errorField  string
		}{
			{
				name: "valid team ID",
				params: map[string]interface{}{
					"team_id": "team-abc123def456",
				},
				expectError: false,
			},
			{
				name:        "missing team ID",
				params:      map[string]interface{}{},
				expectError: true,
				errorField:  "team_id",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				teamID, err := request.RequireString("team_id")

				if tt.expectError {
					switch tt.errorField {
					case "team_id":
						assert.Error(t, err)
					}
				} else {
					assert.NoError(t, err)
					if val, ok := tt.params["team_id"]; ok {
						assert.Equal(t, val, teamID)
					}
				}
			})
		}
	})
}
