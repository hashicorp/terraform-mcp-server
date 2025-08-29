// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetWorkspaceDetails(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetWorkspaceDetails(logger)
		
		assert.Equal(t, "get_workspace_details", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "detailed information about a specific Terraform workspace")
		assert.NotNil(t, tool.Handler)
		
		// Verify it's marked as read-only
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
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
			errorField  string
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

	t.Run("workspace variable structure", func(t *testing.T) {
		variables := []*tfe.Variable{
			{
				ID:          "var-123",
				Key:         "environment",
				Value:       "production",
				Description: "Environment name",
				Category:    tfe.CategoryTerraform,
				HCL:         false,
				Sensitive:   false,
			},
			{
				ID:          "var-456",
				Key:         "database_password",
				Value:       "", // Sensitive values are not included
				Description: "Database password",
				Category:    tfe.CategoryEnv,
				HCL:         false,
				Sensitive:   true,
			},
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(variables)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "var-123")
		assert.Contains(t, string(jsonData), "environment")
		assert.Contains(t, string(jsonData), "production")
		assert.Contains(t, string(jsonData), "database_password")

		// Test JSON unmarshaling
		var unmarshaled []*tfe.Variable
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		require.Len(t, unmarshaled, 2)
		
		// Check first variable
		assert.Equal(t, "var-123", unmarshaled[0].ID)
		assert.Equal(t, "environment", unmarshaled[0].Key)
		assert.Equal(t, "production", unmarshaled[0].Value)
		assert.False(t, unmarshaled[0].Sensitive)
		
		// Check sensitive variable
		assert.Equal(t, "var-456", unmarshaled[1].ID)
		assert.Equal(t, "database_password", unmarshaled[1].Key)
		assert.Empty(t, unmarshaled[1].Value) // Sensitive value should be empty
		assert.True(t, unmarshaled[1].Sensitive)
	})

	t.Run("workspace tags structure", func(t *testing.T) {
		tags := []*tfe.Tag{
			{Name: "environment:production"},
			{Name: "team:backend"},
			{Name: "version:v1.0.0"},
		}

		// Test JSON marshaling of tags
		jsonData, err := json.Marshal(tags)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "environment:production")
		assert.Contains(t, string(jsonData), "team:backend")
		assert.Contains(t, string(jsonData), "version:v1.0.0")

		// Test JSON unmarshaling
		var unmarshaled []*tfe.Tag
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		require.Len(t, unmarshaled, 3)
		assert.Equal(t, "environment:production", unmarshaled[0].Name)
		assert.Equal(t, "team:backend", unmarshaled[1].Name)
		assert.Equal(t, "version:v1.0.0", unmarshaled[2].Name)
	})
}
