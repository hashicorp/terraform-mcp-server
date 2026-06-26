// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestForceUnlockWorkspace(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := ForceUnlockWorkspace(logger)

		// Verify tool identity
		assert.Equal(t, "force_unlock_workspace", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Force unlocks a Terraform workspace stuck in a lock")
		assert.NotNil(t, tool.Handler)

		// Verify it is marked as destructive and not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Verify required parameters are declared in the schema
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_id")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
			errorField  string
		}{
			{
				name: "valid minimal parameters",
				params: map[string]interface{}{
					"workspace_id": "ws-123456",
				},
				expectError: false,
			},
			{
				name: "valid workspace ID with long format",
				params: map[string]interface{}{
					"workspace_id": "ws-abc123def456ghi789",
				},
				expectError: false,
			},
			{
				name: "missing workspace ID",
				params: map[string]interface{}{},
				expectError: true,
				errorField: "workspace_id",
			},
			{
				// Extra fields should be silently ignored by RequireString
				name: "valid parameters with extra fields",
				params: map[string]interface{}{
					"workspace_id": "ws-123456",
					"extra_field":  "should be ignored",
					"another_one":  true,
				},
				expectError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				workspaceID, err := request.RequireString("workspace_id")

				if tt.expectError {
					switch tt.errorField {
					case "workspace_id":
						assert.Error(t, err)
					}
				} else {
					assert.NoError(t, err)
					if val, ok := tt.params["workspace_id"]; ok {
						assert.Equal(t, val, workspaceID)
					}
				}
			})
		}
	})

	t.Run("workspace ID format validation", func(t *testing.T) {
		// TFE workspace IDs always begin with "ws-" followed by at least one character.
		tests := []struct {
			name        string
			workspaceID string
			expectValid bool
		}{
			{"valid workspace ID", "ws-123456789abcdef", true},
			{"valid short workspace ID", "ws-123abc", true},
			{"invalid format - no prefix", "123456789abcdef", false},
			{"invalid format - wrong prefix", "workspace-123456", false},
			{"empty workspace ID", "", false},
			{"only prefix", "ws-", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				isValid := strings.HasPrefix(tt.workspaceID, "ws-") && len(tt.workspaceID) > 3
				assert.Equal(t, tt.expectValid, isValid)
			})
		}
	})
}
