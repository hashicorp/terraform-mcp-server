// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateProject(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateProject(logger)

		assert.Equal(t, "create_project", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform project")
		assert.NotNil(t, tool.Handler)

		// Verify annotations: writes state, not destructive, calls external TFE API
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
		assert.True(t, *tool.Tool.Annotations.OpenWorldHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "project_name")

		// description is optional
		_, hasDescription := tool.Tool.InputSchema.Properties["description"]
		assert.True(t, hasDescription)
		assert.NotContains(t, tool.Tool.InputSchema.Required, "description")

		// default_execution_mode is optional
		_, hasExecMode := tool.Tool.InputSchema.Properties["default_execution_mode"]
		assert.True(t, hasExecMode)
		assert.NotContains(t, tool.Tool.InputSchema.Required, "default_execution_mode")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name          string
			params        map[string]interface{}
			expectOrgErr  bool
			expectNameErr bool
		}{
			{
				name: "both params present",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
					"project_name":       "my-project",
				},
				expectOrgErr:  false,
				expectNameErr: false,
			},
			{
				name: "missing terraform_org_name",
				params: map[string]interface{}{
					"project_name": "my-project",
				},
				expectOrgErr:  true,
				expectNameErr: false,
			},
			{
				name: "missing project_name",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
				},
				expectOrgErr:  false,
				expectNameErr: true,
			},
			{
				name:          "both params missing",
				params:        map[string]interface{}{},
				expectOrgErr:  true,
				expectNameErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, orgErr := request.RequireString("terraform_org_name")
				projectName, nameErr := request.RequireString("project_name")

				if tt.expectOrgErr {
					assert.Error(t, orgErr)
					assert.Contains(t, orgErr.Error(), "terraform_org_name")
				} else {
					assert.NoError(t, orgErr)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
				}

				if tt.expectNameErr {
					assert.Error(t, nameErr)
					assert.Contains(t, nameErr.Error(), "project_name")
				} else {
					assert.NoError(t, nameErr)
					assert.Equal(t, tt.params["project_name"], projectName)
				}
			})
		}
	})
}
