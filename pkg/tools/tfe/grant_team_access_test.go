// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGrantTeamAccess(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GrantTeamAccess(logger)

		assert.Equal(t, "grant_team_access", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Grants a team permission to access a workspace or a project")
		assert.NotNil(t, tool.Handler)

		// Check annotations
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "access_level")

		// Check optional parameters exist in schema
		assert.NotNil(t, tool.Tool.InputSchema.Properties["workspace_id"])
		assert.NotNil(t, tool.Tool.InputSchema.Properties["project_id"])
	})
}

func TestGrantTeamAccessHandler_InputValidation(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)
	_ = logger

	t.Run("missing team_id", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				// team_id intentionally omitted
				"access_level": "read",
				"workspace_id": "ws-abc123",
			},
		}

		_, err := request.RequireString("team_id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required parameter")
	})

	t.Run("missing access_level", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"team_id":      "team-abc123",
				"workspace_id": "ws-abc123",
				// access_level intentionally omitted
			},
		}

		_, err := request.RequireString("access_level")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required parameter")
	})
}
