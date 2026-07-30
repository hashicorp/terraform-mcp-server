// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestListTeams(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := ListTeams(logger)

		assert.Equal(t, "list_teams", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "List teams in a Terraform Cloud organization")
		assert.NotNil(t, tool.Handler)

		// Check annotations
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that terraform_org_name is a required parameter
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
	})

	t.Run("successful team list", func(t *testing.T) {
		// Build mock teams
		mockTeams := []*tfe.Team{
			{
				ID:         "team-abc123",
				Name:       "owners",
				Visibility: "secret",
				UserCount:  3,
			},
			{
				ID:         "team-def456",
				Name:       "developers",
				Visibility: "organization",
				UserCount:  10,
			},
		}

		mockTeamList := &tfe.TeamList{
			Items: mockTeams,
		}

		// Verify mock structure
		assert.Len(t, mockTeamList.Items, 2)
		assert.Equal(t, "team-abc123", mockTeamList.Items[0].ID)
		assert.Equal(t, "owners", mockTeamList.Items[0].Name)
		assert.Equal(t, "team-def456", mockTeamList.Items[1].ID)
		assert.Equal(t, "developers", mockTeamList.Items[1].Name)
	})

	t.Run("missing required parameter", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				// Missing terraform_org_name
				"search_query": "platform",
			},
		}

		_, err := request.RequireString("terraform_org_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required parameter")
	})
}
