// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetStateVersion(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Tool definition contract
	t.Run("tool creation", func(t *testing.T) {
		tool := GetStateVersion(logger)

		assert.Equal(t, "get_state_version", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "Gets StateVersion with state_version_id")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		assert.NotContains(t, tool.Tool.InputSchema.Required, "state_version_id")
		assert.NotContains(t, tool.Tool.InputSchema.Required, "workspace_id")
	})

	// Required parameter validation
	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
		}{
			{
				name:        "param present",
				params:      map[string]interface{}{"state_version_id": "sv-abc123"},
				expectError: false,
			},
			{
				name:        "param missing",
				params:      map[string]interface{}{},
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}
				val, err := request.RequireString("state_version_id")

				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "state_version_id")
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.params["state_version_id"], val)
				}
			})
		}
	})
}
