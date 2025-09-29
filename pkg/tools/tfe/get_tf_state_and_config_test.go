// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetTfStateAndConfig(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetTfStateAndConfig(logger)

		assert.Equal(t, "get_tf_state_and_config", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Fetches both the current Terraform state and configuration")
		assert.NotNil(t, tool.Handler)

		// Verify it's marked as read-only
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		
		// Verify it's not marked as destructive
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
			errorMsg    string
		}{
			{
				name: "valid parameters",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
				},
				expectError: false,
			},
			{
				name: "missing org name",
				params: map[string]interface{}{
					"workspace_name": "test-workspace",
				},
				expectError: true,
				errorMsg:    "terraform_org_name",
			},
			{
				name: "missing workspace name",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
				},
				expectError: true,
				errorMsg:    "workspace_name",
			},
			{
				name: "empty org name",
				params: map[string]interface{}{
					"terraform_org_name": "",
					"workspace_name":     "test-workspace",
				},
				expectError: false, // Empty strings are valid - they get trimmed in the handler
			},
			{
				name: "whitespace org name",
				params: map[string]interface{}{
					"terraform_org_name": "  test-org  ",
					"workspace_name":     "test-workspace",
				},
				expectError: false, // Whitespace gets trimmed in the handler
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, err1 := request.RequireString("terraform_org_name")
				workspaceName, err2 := request.RequireString("workspace_name")

				if tt.expectError {
					if strings.Contains(tt.errorMsg, "terraform_org_name") {
						assert.Error(t, err1)
					}
					if strings.Contains(tt.errorMsg, "workspace_name") {
						assert.Error(t, err2)
					}
				} else {
					if _, ok := tt.params["terraform_org_name"]; ok {
						assert.NoError(t, err1)
						assert.Equal(t, tt.params["terraform_org_name"], orgName)
					}
					if _, ok := tt.params["workspace_name"]; ok {
						assert.NoError(t, err2)
						assert.Equal(t, tt.params["workspace_name"], workspaceName)
					}
				}
			})
		}
	})
}
