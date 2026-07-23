// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestListStateVersions(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Tool definition contract
	t.Run("tool creation", func(t *testing.T) {
		tool := ListStateVersions(logger)

		assert.Equal(t, "list_state_versions", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "List all States Versions")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
	})

	// Required parameter validation
	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name         string
			params       map[string]interface{}
			expectOrgErr bool
			expectWsErr  bool
		}{
			{
				name: "both params present",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
					"workspace_name":     "my-workspace",
				},
				expectOrgErr: false,
				expectWsErr:  false,
			},
			{
				name: "missing terraform_org_name",
				params: map[string]interface{}{
					"workspace_name": "my-workspace",
				},
				expectOrgErr: true,
				expectWsErr:  false,
			},
			{
				name: "missing workspace_name",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
				},
				expectOrgErr: false,
				expectWsErr:  true,
			},
			{
				name:         "both params missing",
				params:       map[string]interface{}{},
				expectOrgErr: true,
				expectWsErr:  true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, orgErr := request.RequireString("terraform_org_name")
				wsName, wsErr := request.RequireString("workspace_name")

				if tt.expectOrgErr {
					assert.Error(t, orgErr)
					assert.Contains(t, orgErr.Error(), "terraform_org_name")
				} else {
					assert.NoError(t, orgErr)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
				}

				if tt.expectWsErr {
					assert.Error(t, wsErr)
					assert.Contains(t, wsErr.Error(), "workspace_name")
				} else {
					assert.NoError(t, wsErr)
					assert.Equal(t, tt.params["workspace_name"], wsName)
				}
			})
		}
	})
}
