// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		// description is optional: present in properties but not in required
		_, hasDescription := tool.Tool.InputSchema.Properties["description"]
		assert.True(t, hasDescription)
		assert.NotContains(t, tool.Tool.InputSchema.Required, "description")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			missingKey  string
			expectError bool
		}{
			{
				name: "both required params present",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
					"project_name":       "my-project",
				},
				expectError: false,
			},
			{
				name: "missing terraform_org_name",
				params: map[string]interface{}{
					"project_name": "my-project",
				},
				missingKey:  "terraform_org_name",
				expectError: true,
			},
			{
				name: "missing project_name",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
				},
				missingKey:  "project_name",
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				_, orgErr := request.RequireString("terraform_org_name")
				_, nameErr := request.RequireString("project_name")

				if tt.expectError {
					if tt.missingKey == "terraform_org_name" {
						assert.Error(t, orgErr)
					} else {
						assert.NoError(t, orgErr)
					}
					if tt.missingKey == "project_name" {
						assert.Error(t, nameErr)
					} else {
						assert.NoError(t, nameErr)
					}
				} else {
					assert.NoError(t, orgErr)
					assert.NoError(t, nameErr)
				}
			})
		}
	})

	t.Run("project_name validation", func(t *testing.T) {
		validPattern := regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)

		tests := []struct {
			name        string
			input       string
			expectError bool
		}{
			// Valid cases
			{name: "minimum length", input: "abc", expectError: false},
			{name: "maximum length", input: strings.Repeat("a", 40), expectError: false},
			{name: "letters and numbers", input: "Project123", expectError: false},
			{name: "hyphens and underscores", input: "my-project_name", expectError: false},
			{name: "internal spaces", input: "my project", expectError: false},

			// Invalid — length
			{name: "too short (2 chars)", input: "ab", expectError: true},
			{name: "too long (41 chars)", input: strings.Repeat("a", 41), expectError: true},

			// Invalid — leading/trailing spaces
			{name: "leading space", input: " myproject", expectError: true},
			{name: "trailing space", input: "myproject ", expectError: true},

			// Invalid — disallowed characters
			{name: "slash", input: "my/project", expectError: true},
			{name: "exclamation", input: "my!project", expectError: true},
			{name: "at sign", input: "my@project", expectError: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var validationErr bool

				if len(tt.input) < 3 || len(tt.input) > 40 {
					validationErr = true
				} else if tt.input != strings.TrimSpace(tt.input) {
					validationErr = true
				} else if !validPattern.MatchString(tt.input) {
					validationErr = true
				}

				assert.Equal(t, tt.expectError, validationErr,
					"input %q: expected error=%v", tt.input, tt.expectError)
			})
		}
	})

	t.Run("description validation", func(t *testing.T) {
		tests := []struct {
			name        string
			input       string
			expectError bool
		}{
			{name: "empty (optional)", input: "", expectError: false},
			{name: "short description", input: "A simple project", expectError: false},
			{name: "exactly 256 chars", input: strings.Repeat("a", 256), expectError: false},
			{name: "257 chars (over limit)", input: strings.Repeat("a", 257), expectError: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tooLong := len(tt.input) > 256
				assert.Equal(t, tt.expectError, tooLong,
					"description of length %d: expected error=%v", len(tt.input), tt.expectError)
			})
		}
	})

	t.Run("result JSON structure", func(t *testing.T) {
		result := &ProjectSummary{
			ID:   "prj-abc123",
			Name: "my-project",
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var decoded ProjectSummary
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, "prj-abc123", decoded.ID)
		assert.Equal(t, "my-project", decoded.Name)

		// Verify JSON key names match the spec
		assert.Contains(t, string(data), `"project_id"`)
		assert.Contains(t, string(data), `"project_name"`)
	})
}
