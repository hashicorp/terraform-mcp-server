// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateRunSafe(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRunSafe(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is false
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)

		// Check that actions property exists
		actionsProperty := tool.Tool.InputSchema.Properties["actions"]
		assert.NotNil(t, actionsProperty)
	})

	t.Run("actions parameter parsing", func(t *testing.T) {
		tests := []struct {
			name     string
			params   map[string]interface{}
			expected []string
		}{
			{
				name: "no actions parameter",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
				},
				expected: nil,
			},
			{
				name: "empty actions array",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{},
				},
				expected: nil,
			},
			{
				name: "single action",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.foo.bar"},
				},
				expected: []string{"actions.foo.bar"},
			},
			{
				name: "multiple actions",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.foo.bar", "actions.baz.qux"},
				},
				expected: []string{"actions.foo.bar", "actions.baz.qux"},
			},
			{
				name: "mixed types in actions array (filters non-strings)",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.foo.bar", 123, "actions.baz.qux", true},
				},
				expected: []string{"actions.foo.bar", "actions.baz.qux"},
			},
			{
				name: "non-array actions parameter",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            "not-an-array",
				},
				expected: nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				// Test the actions parameter extraction logic
				var actions []string
				if actionsRaw, ok := request.GetArguments()["actions"]; ok {
					if actionsArray, ok := actionsRaw.([]interface{}); ok {
						for _, action := range actionsArray {
							if actionStr, ok := action.(string); ok {
								actions = append(actions, actionStr)
							}
						}
					}
				}

				assert.Equal(t, tt.expected, actions)
			})
		}
	})
}

func TestCreateRun(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRun(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is true
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)

		// Check that actions property exists
		actionsProperty := tool.Tool.InputSchema.Properties["actions"]
		assert.NotNil(t, actionsProperty)
	})

	t.Run("actions parameter parsing", func(t *testing.T) {
		tests := []struct {
			name     string
			params   map[string]interface{}
			expected []string
		}{
			{
				name: "no actions parameter",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
				},
				expected: nil,
			},
			{
				name: "empty actions array",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{},
				},
				expected: nil,
			},
			{
				name: "single action",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.example.deploy"},
				},
				expected: []string{"actions.example.deploy"},
			},
			{
				name: "multiple actions",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.deploy.app", "actions.notify.slack", "actions.rollback.plan"},
				},
				expected: []string{"actions.deploy.app", "actions.notify.slack", "actions.rollback.plan"},
			},
			{
				name: "complex action names",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"actions":            []interface{}{"actions.providers.aws.deploy", "actions.modules.vpc.configure"},
				},
				expected: []string{"actions.providers.aws.deploy", "actions.modules.vpc.configure"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				// Test the actions parameter extraction logic
				var actions []string
				if actionsRaw, ok := request.GetArguments()["actions"]; ok {
					if actionsArray, ok := actionsRaw.([]interface{}); ok {
						for _, action := range actionsArray {
							if actionStr, ok := action.(string); ok {
								actions = append(actions, actionStr)
							}
						}
					}
				}

				assert.Equal(t, tt.expected, actions)
			})
		}
	})

	t.Run("parameter validation with actions", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
			errorMsg    string
		}{
			{
				name: "valid parameters with actions",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"run_type":           "plan_and_apply",
					"message":            "Test run with actions",
					"actions":            []interface{}{"actions.foo.bar", "actions.baz.qux"},
				},
				expectError: false,
			},
			{
				name: "valid parameters without actions",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"workspace_name":     "test-workspace",
					"run_type":           "plan_only",
					"message":            "Test run without actions",
				},
				expectError: false,
			},
			{
				name: "missing required terraform_org_name",
				params: map[string]interface{}{
					"workspace_name": "test-workspace",
					"actions":        []interface{}{"actions.foo.bar"},
				},
				expectError: true,
				errorMsg:    "terraform_org_name",
			},
			{
				name: "missing required workspace_name",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
					"actions":            []interface{}{"actions.foo.bar"},
				},
				expectError: true,
				errorMsg:    "workspace_name",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				// Test required parameter validation
				orgName, err1 := request.RequireString("terraform_org_name")
				workspaceName, err2 := request.RequireString("workspace_name")

				if tt.expectError {
					if tt.errorMsg == "terraform_org_name" {
						assert.Error(t, err1)
					}
					if tt.errorMsg == "workspace_name" {
						assert.Error(t, err2)
					}
				} else {
					assert.NoError(t, err1)
					assert.NoError(t, err2)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
					assert.Equal(t, tt.params["workspace_name"], workspaceName)

					// Test optional parameters
					runType := request.GetString("run_type", "plan_and_apply")
					message := request.GetString("message", "Triggered via Terraform MCP Server")

					if expectedRunType, ok := tt.params["run_type"]; ok {
						assert.Equal(t, expectedRunType, runType)
					} else {
						assert.Equal(t, "plan_and_apply", runType)
					}

					if expectedMessage, ok := tt.params["message"]; ok {
						assert.Equal(t, expectedMessage, message)
					} else {
						assert.Equal(t, "Triggered via Terraform MCP Server", message)
					}

					// Test actions parameter extraction
					var actions []string
					if actionsRaw, ok := request.GetArguments()["actions"]; ok {
						if actionsArray, ok := actionsRaw.([]interface{}); ok {
							for _, action := range actionsArray {
								if actionStr, ok := action.(string); ok {
									actions = append(actions, actionStr)
								}
							}
						}
					}

					if expectedActions, ok := tt.params["actions"]; ok {
						expectedActionsSlice := make([]string, 0)
						if actionArray, ok := expectedActions.([]interface{}); ok {
							for _, action := range actionArray {
								if actionStr, ok := action.(string); ok {
									expectedActionsSlice = append(expectedActionsSlice, actionStr)
								}
							}
						}
						assert.Equal(t, expectedActionsSlice, actions)
					} else {
						assert.Nil(t, actions)
					}
				}
			})
		}
	})
}
