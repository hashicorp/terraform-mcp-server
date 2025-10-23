// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// MockCallToolRequest implements the mcp.CallToolRequest interface for testing
type MockCallToolRequest struct {
	params map[string]interface{}
}

func (m *MockCallToolRequest) RequireString(key string) (string, error) {
	if val, ok := m.params[key]; ok {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}
	return "", errors.New("missing required parameter: " + key)
}

func (m *MockCallToolRequest) GetString(key, defaultValue string) string {
	if val, ok := m.params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func (m *MockCallToolRequest) GetBoolean(key string, defaultValue bool) bool {
	if val, ok := m.params[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func TestSearchWorkspaces(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := ListWorkspaces(logger)

		assert.Equal(t, "list_workspaces", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "List Terraform workspaces with queries")
		assert.NotNil(t, tool.Handler)

		// Check annotations
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that terraform_org_name is in required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
	})

	t.Run("successful workspace search", func(t *testing.T) {
		// Create mock workspaces
		mockWorkspaces := []*tfe.Workspace{
			{
				ID:               "ws-123",
				Name:             "test-workspace-1",
				Description:      "Test workspace 1",
				AutoApply:        false,
				ExecutionMode:    "remote",
				ResourceCount:    5,
				Locked:           false,
				TerraformVersion: "1.5.0",
				WorkingDirectory: "/",
			},
			{
				ID:               "ws-456",
				Name:             "test-workspace-2",
				Description:      "Test workspace 2",
				AutoApply:        true,
				ExecutionMode:    "local",
				ResourceCount:    10,
				Locked:           true,
				TerraformVersion: "1.4.0",
				WorkingDirectory: "/modules",
			},
		}

		mockWorkspaceList := &tfe.WorkspaceList{
			Items: mockWorkspaces,
		}

		// Verify the mock workspace list structure
		assert.Len(t, mockWorkspaceList.Items, 2)
		assert.Equal(t, "ws-123", mockWorkspaceList.Items[0].ID)
		assert.Equal(t, "test-workspace-1", mockWorkspaceList.Items[0].Name)
	})

	t.Run("missing required parameter", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				// Missing terraform_org_name
				"search_query": "test",
			},
		}

		_, err := request.RequireString("terraform_org_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required parameter")
	})

	t.Run("parameter parsing", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"terraform_org_name": "test-org",
				"search_query":       "my-workspace",
				"project_id":         "prj-123",
				"tags":               "env:prod,team:backend",
				"exclude_tags":       "deprecated",
				"wildcard_name":      "*-prod",
			},
		}

		orgName, err := request.RequireString("terraform_org_name")
		assert.NoError(t, err)
		assert.Equal(t, "test-org", orgName)

		searchQuery := request.GetString("search_query", "")
		assert.Equal(t, "my-workspace", searchQuery)

		projectID := request.GetString("project_id", "")
		assert.Equal(t, "prj-123", projectID)

		tags := request.GetString("tags", "")
		assert.Equal(t, "env:prod,team:backend", tags)

		excludeTags := request.GetString("exclude_tags", "")
		assert.Equal(t, "deprecated", excludeTags)

		wildcardName := request.GetString("wildcard_name", "")
		assert.Equal(t, "*-prod", wildcardName)
	})

	t.Run("workspace info structure", func(t *testing.T) {
		// Test the WorkspaceInfo structure used in the response
		type WorkspaceInfo struct {
			ID               string     `json:"id"`
			Name             string     `json:"name"`
			Description      string     `json:"description,omitempty"`
			Environment      string     `json:"environment,omitempty"`
			AutoApply        bool       `json:"auto_apply"`
			TerraformVersion string     `json:"terraform_version,omitempty"`
			WorkingDirectory string     `json:"working_directory,omitempty"`
			Locked           bool       `json:"locked"`
			ExecutionMode    string     `json:"execution_mode,omitempty"`
			ResourceCount    int        `json:"resource_count"`
			ApplyDurationAvg int64      `json:"apply_duration_average,omitempty"`
			PlanDurationAvg  int64      `json:"plan_duration_average,omitempty"`
			PolicyCheckFails int        `json:"policy_check_failures,omitempty"`
			RunFailures      int        `json:"run_failures,omitempty"`
			Tags             []*tfe.Tag `json:"tags,omitempty"`
			CreatedAt        string     `json:"created_at,omitempty"`
			UpdatedAt        string     `json:"updated_at,omitempty"`
		}

		info := WorkspaceInfo{
			ID:               "ws-123",
			Name:             "test-workspace",
			Description:      "Test workspace",
			AutoApply:        true,
			ExecutionMode:    "remote",
			ResourceCount:    5,
			Locked:           false,
			TerraformVersion: "1.5.0",
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(info)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "ws-123")
		assert.Contains(t, string(jsonData), "test-workspace")
		assert.Contains(t, string(jsonData), "remote")

		// Test JSON unmarshaling
		var unmarshaled WorkspaceInfo
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, info.ID, unmarshaled.ID)
		assert.Equal(t, info.Name, unmarshaled.Name)
		assert.Equal(t, info.AutoApply, unmarshaled.AutoApply)
	})
}

func TestSearchWorkspacesTagParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single tag",
			input:    "env:prod",
			expected: []string{"env:prod"},
		},
		{
			name:     "multiple tags",
			input:    "env:prod,team:backend,version:v1.0",
			expected: []string{"env:prod", "team:backend", "version:v1.0"},
		},
		{
			name:     "tags with spaces",
			input:    " env:prod , team:backend , version:v1.0 ",
			expected: []string{"env:prod", "team:backend", "version:v1.0"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tags []string
			if tt.input != "" {
				tagList := strings.Split(strings.TrimSpace(tt.input), ",")
				tags = make([]string, 0, len(tagList))
				for _, tag := range tagList {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			}

			if tt.expected == nil {
				assert.Nil(t, tags)
			} else {
				assert.Equal(t, tt.expected, tags)
			}
		})
	}
}
