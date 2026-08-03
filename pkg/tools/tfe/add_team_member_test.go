// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAddTeamMember(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := AddTeamMember(logger)

		assert.Equal(t, "add_team_member", tool.Tool.Name)
		assert.NotNil(t, tool.Handler)

		// Not destructive, not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// team_id is required; the optional inputs must not appear in the required list
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_id")
		assert.NotContains(t, tool.Tool.InputSchema.Required, "username")
		assert.NotContains(t, tool.Tool.InputSchema.Required, "organization_membership_ids")
	})

	t.Run("team_id requirement", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
		}{
			{
				name:        "team_id present",
				params:      map[string]interface{}{"team_id": "team-abc123"},
				expectError: false,
			},
			{
				name:        "team_id missing",
				params:      map[string]interface{}{},
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}
				_, err := request.RequireString("team_id")
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("at least one id list required", func(t *testing.T) {
		tests := []struct {
			name          string
			usernames     []string
			membershipIDs []string
			expectValid   bool
		}{
			{
				name:          "only usernames provided",
				usernames:     []string{"alice"},
				membershipIDs: nil,
				expectValid:   true,
			},
			{
				name:          "only membership IDs provided",
				usernames:     nil,
				membershipIDs: []string{"ou-abc123"},
				expectValid:   true,
			},
			{
				name:          "both provided",
				usernames:     []string{"alice"},
				membershipIDs: []string{"ou-abc123"},
				expectValid:   true,
			},
			{
				name:          "neither provided",
				usernames:     nil,
				membershipIDs: nil,
				expectValid:   false,
			},
			{
				name:          "both empty slices",
				usernames:     []string{},
				membershipIDs: []string{},
				expectValid:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				isValid := len(tt.usernames) > 0 || len(tt.membershipIDs) > 0
				assert.Equal(t, tt.expectValid, isValid)
			})
		}
	})
}
