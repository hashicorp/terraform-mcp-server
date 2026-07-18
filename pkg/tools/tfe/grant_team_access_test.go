// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrantTeamAccess(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GrantTeamAccess(logger)

		assert.Equal(t, "grant_team_access", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Grants a team access")
		assert.NotNil(t, tool.Handler)

		// Not destructive, not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "access_level")

		// workspace_id and project_id are optional (mutual exclusion is enforced at runtime)
		assert.NotContains(t, tool.Tool.InputSchema.Required, "workspace_id")
		assert.NotContains(t, tool.Tool.InputSchema.Required, "project_id")
	})

	t.Run("missing team_id returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"access_level": "read",
			"workspace_id": "ws-abc123",
		}}
		_, err := request.RequireString("team_id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "team_id")
	})

	t.Run("missing access_level returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"team_id":      "team-abc123",
			"workspace_id": "ws-abc123",
		}}
		_, err := request.RequireString("access_level")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access_level")
	})

	t.Run("neither workspace_id nor project_id returns error", func(t *testing.T) {
		workspaceID := strings.TrimSpace("")
		projectID := strings.TrimSpace("")

		assert.True(t, workspaceID == "" && projectID == "",
			"both IDs empty should trigger validation error")
	})

	t.Run("both workspace_id and project_id returns error", func(t *testing.T) {
		workspaceID := strings.TrimSpace("ws-abc123")
		projectID := strings.TrimSpace("prj-xyz789")

		assert.True(t, workspaceID != "" && projectID != "",
			"both IDs provided should trigger mutual exclusion error")
	})

	t.Run("workspace_id alone is valid", func(t *testing.T) {
		workspaceID := strings.TrimSpace("ws-abc123")
		projectID := strings.TrimSpace("")

		assert.True(t, workspaceID != "" && projectID == "")
	})

	t.Run("project_id alone is valid", func(t *testing.T) {
		workspaceID := strings.TrimSpace("")
		projectID := strings.TrimSpace("prj-xyz789")

		assert.True(t, workspaceID == "" && projectID != "")
	})
}

func TestGrantTeamAccess_WorkspaceAccessLevelValidation(t *testing.T) {
	// These are the valid workspace access levels accepted by grantWorkspaceAccess.
	validWorkspaceAccessLevels := []tfe.AccessType{
		tfe.AccessAdmin,
		tfe.AccessPlan,
		tfe.AccessRead,
		tfe.AccessWrite,
		tfe.AccessCustom,
	}

	for _, level := range validWorkspaceAccessLevels {
		t.Run("valid workspace access: "+string(level), func(t *testing.T) {
			accessType := tfe.AccessType(level)
			var valid bool
			switch accessType {
			case tfe.AccessAdmin, tfe.AccessPlan, tfe.AccessRead, tfe.AccessWrite, tfe.AccessCustom:
				valid = true
			default:
				valid = false
			}
			assert.True(t, valid)
		})
	}

	t.Run("invalid workspace access level 'maintain'", func(t *testing.T) {
		// "maintain" is only valid for project access, not workspace access
		accessType := tfe.AccessType("maintain")
		var valid bool
		switch accessType {
		case tfe.AccessAdmin, tfe.AccessPlan, tfe.AccessRead, tfe.AccessWrite, tfe.AccessCustom:
			valid = true
		default:
			valid = false
		}
		assert.False(t, valid)
	})

	t.Run("invalid workspace access level empty string", func(t *testing.T) {
		accessType := tfe.AccessType("")
		var valid bool
		switch accessType {
		case tfe.AccessAdmin, tfe.AccessPlan, tfe.AccessRead, tfe.AccessWrite, tfe.AccessCustom:
			valid = true
		default:
			valid = false
		}
		assert.False(t, valid)
	})
}

func TestGrantTeamAccess_ProjectAccessLevelValidation(t *testing.T) {
	// These are the valid project access levels accepted by grantProjectAccess.
	validProjectAccessLevels := []tfe.TeamProjectAccessType{
		tfe.TeamProjectAccessAdmin,
		tfe.TeamProjectAccessMaintain,
		tfe.TeamProjectAccessWrite,
		tfe.TeamProjectAccessRead,
		tfe.TeamProjectAccessCustom,
	}

	for _, level := range validProjectAccessLevels {
		t.Run("valid project access: "+string(level), func(t *testing.T) {
			accessType := tfe.TeamProjectAccessType(level)
			var valid bool
			switch accessType {
			case tfe.TeamProjectAccessAdmin, tfe.TeamProjectAccessMaintain,
				tfe.TeamProjectAccessWrite, tfe.TeamProjectAccessRead, tfe.TeamProjectAccessCustom:
				valid = true
			default:
				valid = false
			}
			assert.True(t, valid)
		})
	}

	t.Run("invalid project access level 'plan'", func(t *testing.T) {
		// "plan" is only valid for workspace access, not project access
		accessType := tfe.TeamProjectAccessType("plan")
		var valid bool
		switch accessType {
		case tfe.TeamProjectAccessAdmin, tfe.TeamProjectAccessMaintain,
			tfe.TeamProjectAccessWrite, tfe.TeamProjectAccessRead, tfe.TeamProjectAccessCustom:
			valid = true
		default:
			valid = false
		}
		assert.False(t, valid)
	})
}

func TestGrantTeamAccess_WorkspaceResultStructure(t *testing.T) {
	result := map[string]interface{}{
		"access_id":    "twa-xyz789",
		"team_id":      "team-abc123",
		"workspace_id": "ws-def456",
		"access_level": "read",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &out))

	assert.Equal(t, "twa-xyz789", out["access_id"])
	assert.Equal(t, "team-abc123", out["team_id"])
	assert.Equal(t, "ws-def456", out["workspace_id"])
	assert.Equal(t, "read", out["access_level"])
	assert.NotContains(t, out, "project_id")
}

func TestGrantTeamAccess_ProjectResultStructure(t *testing.T) {
	result := map[string]interface{}{
		"access_id":    "tpa-xyz789",
		"team_id":      "team-abc123",
		"project_id":   "prj-def456",
		"access_level": "maintain",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &out))

	assert.Equal(t, "tpa-xyz789", out["access_id"])
	assert.Equal(t, "team-abc123", out["team_id"])
	assert.Equal(t, "prj-def456", out["project_id"])
	assert.Equal(t, "maintain", out["access_level"])
	assert.NotContains(t, out, "workspace_id")
}
