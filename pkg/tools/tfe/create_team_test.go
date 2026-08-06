// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateTeam(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateTeam(logger)

		assert.Equal(t, "create_team", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new team")
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
		assert.Contains(t, tool.Tool.InputSchema.Required, "team_name")

		// visibility is optional
		_, hasVisibility := tool.Tool.InputSchema.Properties["visibility"]
		assert.True(t, hasVisibility)
		assert.NotContains(t, tool.Tool.InputSchema.Required, "visibility")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name          string
			params        map[string]interface{}
			expectOrgErr  bool
			expectTeamErr bool
		}{
			{
				name: "both params present",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
					"team_name":          "my-team",
				},
				expectOrgErr:  false,
				expectTeamErr: false,
			},
			{
				name: "missing terraform_org_name",
				params: map[string]interface{}{
					"team_name": "my-team",
				},
				expectOrgErr:  true,
				expectTeamErr: false,
			},
			{
				name: "missing team_name",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
				},
				expectOrgErr:  false,
				expectTeamErr: true,
			},
			{
				name:          "both params missing",
				params:        map[string]interface{}{},
				expectOrgErr:  true,
				expectTeamErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, orgErr := request.RequireString("terraform_org_name")
				teamName, teamErr := request.RequireString("team_name")

				if tt.expectOrgErr {
					assert.Error(t, orgErr)
					assert.Contains(t, orgErr.Error(), "terraform_org_name")
				} else {
					assert.NoError(t, orgErr)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
				}

				if tt.expectTeamErr {
					assert.Error(t, teamErr)
					assert.Contains(t, teamErr.Error(), "team_name")
				} else {
					assert.NoError(t, teamErr)
					assert.Equal(t, tt.params["team_name"], teamName)
				}
			})
		}
	})
}
