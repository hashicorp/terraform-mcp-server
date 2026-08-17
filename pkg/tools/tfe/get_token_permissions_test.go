// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetTokenPermissions(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetTokenPermissions(logger)

		assert.Equal(t, "get_token_permissions", tool.Tool.Name)
		assert.NotNil(t, tool.Handler)

		// Verify it's marked as read-only
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
	})
}

func TestHumanReadableTokenPermissions(t *testing.T) {
	t.Run("returns enabled permissions", func(t *testing.T) {
		permissions := &tfe.OrganizationPermissions{
			CanCreateTeam: true,
			CanDestroy:    true,
		}

		result := HumanReadableTokenPermissions(permissions)

		assert.ElementsMatch(
			t,
			[]string{"Create Teams", "Destroy"},
			result,
		)
	})
}
