// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDeleteProject(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := DeleteProject(logger)

		assert.Equal(t, "delete_project", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Deletes a Terraform project by ID. This is a destructive operation.")
		assert.NotNil(t, tool.Handler)

		// Verify it's marked as destructive and not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check that the required parameter is defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "project_id")
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
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.params["project_id"], projectID)
				}
			})
		}
	})
}
