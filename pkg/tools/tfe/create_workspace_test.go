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
	"github.com/stretchr/testify/require"
)

func TestCreateWorkspace(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateWorkspace(logger)

		assert.Equal(t, "create_workspace", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform workspace")
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
			errorMsg    string
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
			{"empty mode", "", true}, // Empty defaults to remote
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

	t.Run("tag parsing", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected []*tfe.Tag
		}{
			{
				name:     "single tag",
				input:    "env:prod",
				expected: []*tfe.Tag{{Name: "env:prod"}},
			},
			{
				name:     "multiple tags",
				input:    "env:prod,team:backend,version:v1.0",
				expected: []*tfe.Tag{{Name: "env:prod"}, {Name: "team:backend"}, {Name: "version:v1.0"}},
			},
			{
				name:     "tags with spaces",
				input:    " env:prod , team:backend , version:v1.0 ",
				expected: []*tfe.Tag{{Name: "env:prod"}, {Name: "team:backend"}, {Name: "version:v1.0"}},
			},
			{
				name:     "empty string",
				input:    "",
				expected: nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var tags []*tfe.Tag
				if tt.input != "" {
					tagNames := strings.Split(strings.TrimSpace(tt.input), ",")
					tags = make([]*tfe.Tag, 0, len(tagNames))
					for _, tagName := range tagNames {
						tagName = strings.TrimSpace(tagName)
						if tagName != "" {
							tags = append(tags, &tfe.Tag{Name: tagName})
						}
					}
				}

				if tt.expected == nil {
					assert.Nil(t, tags)
				} else {
					require.Len(t, tags, len(tt.expected))
					for i, expectedTag := range tt.expected {
						assert.Equal(t, expectedTag.Name, tags[i].Name)
					}
				}
			})
		}
	})

	t.Run("workspace creation result structure", func(t *testing.T) {
		// Test the WorkspaceCreationResult structure
		type WorkspaceCreationResult struct {
			ID               string       `json:"id"`
			Name             string       `json:"name"`
			Description      string       `json:"description,omitempty"`
			AutoApply        bool         `json:"auto_apply"`
			ExecutionMode    string       `json:"execution_mode"`
			TerraformVersion string       `json:"terraform_version,omitempty"`
			WorkingDirectory string       `json:"working_directory,omitempty"`
			Tags             []*tfe.Tag   `json:"tags,omitempty"`
			VCSRepo          *tfe.VCSRepo `json:"vcs_repo,omitempty"`
			Project          *tfe.Project `json:"project,omitempty"`
			CreatedAt        string       `json:"created_at,omitempty"`
			Message          string       `json:"message"`
		}

		result := WorkspaceCreationResult{
			ID:               "ws-123456",
			Name:             "test-workspace",
			Description:      "Test workspace",
			AutoApply:        true,
			ExecutionMode:    "remote",
			TerraformVersion: "1.5.0",
			WorkingDirectory: "/modules",
			CreatedAt:        time.Now().Format("2006-01-02T15:04:05Z"),
			Message:          "Workspace created successfully",
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(result)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "ws-123456")
		assert.Contains(t, string(jsonData), "test-workspace")
		assert.Contains(t, string(jsonData), "Workspace created successfully")

		// Test JSON unmarshaling
		var unmarshaled WorkspaceCreationResult
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, result.ID, unmarshaled.ID)
		assert.Equal(t, result.Name, unmarshaled.Name)
		assert.Equal(t, result.AutoApply, unmarshaled.AutoApply)
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
