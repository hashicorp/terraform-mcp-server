// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetProject(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GetProject(logger)

		assert.Equal(t, "get_project", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Terraform project")
		assert.NotNil(t, tool.Handler)

		// Verify annotations: read-only, not destructive, calls external TFE API
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
		assert.True(t, *tool.Tool.Annotations.OpenWorldHint)

		// Check that project_id is the only required parameter
		assert.Contains(t, tool.Tool.InputSchema.Required, "project_id")
		assert.Len(t, tool.Tool.InputSchema.Required, 1)
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
		}{
			{
				name: "valid project_id",
				params: map[string]interface{}{
					"project_id": "prj-abc123def456",
				},
				expectError: false,
			},
			{
				name:        "missing project_id",
				params:      map[string]interface{}{},
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				projectID, err := request.RequireString("project_id")

				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "project_id")
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.params["project_id"], projectID)
				}
			})
		}
	})
}
