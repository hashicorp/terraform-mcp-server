// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDeleteTeam(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

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

	t.Run("team ID format validation", func(t *testing.T) {
		tests := []struct {
			name        string
			teamID      string
			expectValid bool
		}{
			{"valid team ID", "team-abc123def456", true},
			{"valid short team ID", "team-abc123", true},
			{"invalid format - no prefix", "abc123def456", false},
			{"invalid format - wrong prefix", "workspace-abc123", false},
			{"empty team ID", "", false},
			{"only prefix", "team-", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Simple validation: team ID should start with "team-" and have content after
				isValid := strings.HasPrefix(tt.teamID, "team-") && len(tt.teamID) > 5
				assert.Equal(t, tt.expectValid, isValid)
			})
		}
	})

	t.Run("team deletion result structure", func(t *testing.T) {
		type TeamDeletionResult struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			TeamID  string `json:"team_id"`
		}

		result := TeamDeletionResult{
			Success: true,
			Message: `team "my-platform-team" (team-abc123def456) deleted successfully`,
			TeamID:  "team-abc123def456",
		}

		jsonData, err := json.Marshal(result)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "team-abc123def456")
		assert.Contains(t, string(jsonData), "deleted successfully")
		assert.Contains(t, string(jsonData), `"success":true`)

		var unmarshaled TeamDeletionResult
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, result.Success, unmarshaled.Success)
		assert.Equal(t, result.TeamID, unmarshaled.TeamID)
		assert.Equal(t, result.Message, unmarshaled.Message)
	})
}
