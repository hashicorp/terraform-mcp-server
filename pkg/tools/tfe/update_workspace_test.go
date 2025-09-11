// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestUpdateWorkspace(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := UpdateWorkspace(logger)

		assert.Equal(t, "update_workspace", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Updates an existing Terraform workspace")
		assert.NotNil(t, tool.Handler)

		// Verify it's not marked as destructive
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
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
				errorField:  "terraform_org_name",
			},
			{
				name: "missing workspace name",
				params: map[string]interface{}{
					"terraform_org_name": "test-org",
				},
				expectError: true,
				errorField:  "workspace_name",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, err1 := request.RequireString("terraform_org_name")
				workspaceName, err2 := request.RequireString("workspace_name")

				if tt.expectError {
					switch tt.errorField {
					case "terraform_org_name":
						assert.Error(t, err1)
					case "workspace_name":
						assert.Error(t, err2)
					}
				} else {
					assert.NoError(t, err1)
					assert.NoError(t, err2)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
					assert.Equal(t, tt.params["workspace_name"], workspaceName)
				}
			})
		}
	})

	t.Run("execution mode validation", func(t *testing.T) {
		tests := []struct {
			name          string
			executionMode string
			expectValid   bool
		}{
			{"remote mode", "remote", true},
			{"local mode", "local", true},
			{"agent mode", "agent", true},
			{"Remote mode (case insensitive)", "Remote", true},
			{"LOCAL mode (case insensitive)", "LOCAL", true},
			{"invalid mode", "invalid", false},
			{"empty mode", "", true}, // Empty is valid (no update)
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mode := strings.ToLower(tt.executionMode)
				var isValid bool

				switch mode {
				case "local", "agent", "remote", "":
					isValid = true
				default:
					isValid = false
				}

				assert.Equal(t, tt.expectValid, isValid)
			})
		}
	})

	t.Run("workspace update result structure", func(t *testing.T) {
		// Test the WorkspaceUpdateResult structure
		type WorkspaceUpdateResult struct {
			ID                  string     `json:"id"`
			Name                string     `json:"name"`
			Description         string     `json:"description,omitempty"`
			AutoApply           bool       `json:"auto_apply"`
			ExecutionMode       string     `json:"execution_mode"`
			TerraformVersion    string     `json:"terraform_version,omitempty"`
			WorkingDirectory    string     `json:"working_directory,omitempty"`
			QueueAllRuns        bool       `json:"queue_all_runs"`
			SpeculativeEnabled  bool       `json:"speculative_enabled"`
			FileTriggersEnabled bool       `json:"file_triggers_enabled"`
			TriggerPrefixes     []string   `json:"trigger_prefixes,omitempty"`
			Tags                []*tfe.Tag `json:"tags,omitempty"`
			UpdatedAt           string     `json:"updated_at,omitempty"`
			Message             string     `json:"message"`
		}

		result := WorkspaceUpdateResult{
			ID:                  "ws-123456",
			Name:                "updated-workspace",
			Description:         "Updated workspace description",
			AutoApply:           true,
			ExecutionMode:       "local",
			TerraformVersion:    "1.6.0",
			WorkingDirectory:    "/updated-modules",
			QueueAllRuns:        false,
			SpeculativeEnabled:  true,
			FileTriggersEnabled: false,
			TriggerPrefixes:     []string{"modules/", "environments/"},
			UpdatedAt:           time.Now().Format("2006-01-02T15:04:05Z"),
			Message:             "Workspace updated successfully",
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(result)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "ws-123456")
		assert.Contains(t, string(jsonData), "updated-workspace")
		assert.Contains(t, string(jsonData), "Updated workspace description")
		assert.Contains(t, string(jsonData), "local")
		assert.Contains(t, string(jsonData), "Workspace updated successfully")

		// Test JSON unmarshaling
		var unmarshaled WorkspaceUpdateResult
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, result.ID, unmarshaled.ID)
		assert.Equal(t, result.Name, unmarshaled.Name)
		assert.Equal(t, result.AutoApply, unmarshaled.AutoApply)
		assert.Equal(t, result.ExecutionMode, unmarshaled.ExecutionMode)
		assert.Equal(t, result.TriggerPrefixes, unmarshaled.TriggerPrefixes)
		assert.Equal(t, result.Message, unmarshaled.Message)
	})

	t.Run("boolean parameter parsing", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected bool
		}{
			{"true string", "true", true},
			{"True string", "True", true},
			{"TRUE string", "TRUE", true},
			{"false string", "false", false},
			{"False string", "False", false},
			{"FALSE string", "FALSE", false},
			{"empty string", "", false},
			{"invalid string", "invalid", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := strings.ToLower(tt.input) == "true"
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}
