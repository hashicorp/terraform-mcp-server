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

	t.Run("neither workspace_id nor project_id provided", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"team_id":      "team-abc123",
				"access_level": "read",
				// neither workspace_id nor project_id provided
			},
		}

		workspaceID := request.GetString("workspace_id", "")
		projectID := request.GetString("project_id", "")

		assert.Empty(t, workspaceID)
		assert.Empty(t, projectID)
	})

	t.Run("both workspace_id and project_id provided", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"team_id":      "team-abc123",
				"access_level": "read",
				"workspace_id": "ws-abc123",
				"project_id":   "prj-abc123",
			},
		}

		workspaceID := request.GetString("workspace_id", "")
		projectID := request.GetString("project_id", "")

		assert.NotEmpty(t, workspaceID)
		assert.NotEmpty(t, projectID)
	})

	t.Run("parameter parsing", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"team_id":      "team-abc123",
				"access_level": "admin",
				"workspace_id": "ws-abc123",
			},
		}

		teamID, err := request.RequireString("team_id")
		assert.NoError(t, err)
		assert.Equal(t, "team-abc123", teamID)

		accessLevel, err := request.RequireString("access_level")
		assert.NoError(t, err)
		assert.Equal(t, "admin", accessLevel)

		workspaceID := request.GetString("workspace_id", "")
		assert.Equal(t, "ws-abc123", workspaceID)

		projectID := request.GetString("project_id", "")
		assert.Empty(t, projectID)
	})
}

func TestGrantTeamAccessHandler_AccessLevelValidation(t *testing.T) {
	t.Run("valid workspace access levels", func(t *testing.T) {
		validLevels := []string{"admin", "read", "write", "plan", "custom"}
		for _, level := range validLevels {
			found := false
			for _, v := range validTeamAccessLevels {
				if v == level {
					found = true
					break
				}
			}
			assert.True(t, found, "expected %q to be a valid workspace access level", level)
		}
	})

	t.Run("valid project access levels", func(t *testing.T) {
		validLevels := []string{"admin", "read", "write", "maintain", "custom"}
		for _, level := range validLevels {
			found := false
			for _, v := range validTeamProjectAccessLevels {
				if v == level {
					found = true
					break
				}
			}
			assert.True(t, found, "expected %q to be a valid project access level", level)
		}
	})

	t.Run("plan is invalid for project access", func(t *testing.T) {
		found := false
		for _, v := range validTeamProjectAccessLevels {
			if v == "plan" {
				found = true
				break
			}
		}
		assert.False(t, found, `"plan" must not be a valid project access level`)
	})

	t.Run("maintain is invalid for workspace access", func(t *testing.T) {
		found := false
		for _, v := range validTeamAccessLevels {
			if v == "maintain" {
				found = true
				break
			}
		}
		assert.False(t, found, `"maintain" must not be a valid workspace access level`)
	})

	t.Run("invalid access level is not in either list", func(t *testing.T) {
		invalidLevel := "superadmin"
		foundInWorkspace := false
		foundInProject := false
		for _, v := range validTeamAccessLevels {
			if v == invalidLevel {
				foundInWorkspace = true
			}
		}
		for _, v := range validTeamProjectAccessLevels {
			if v == invalidLevel {
				foundInProject = true
			}
		}
		assert.False(t, foundInWorkspace)
		assert.False(t, foundInProject)
	})
}
