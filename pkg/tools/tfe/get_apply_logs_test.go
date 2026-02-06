// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetApplyLogs(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetApplyLogs(logger)

		assert.Equal(t, "get_apply_logs", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "logs")
		assert.Contains(t, tool.Tool.Description, "Terraform apply")
		assert.NotNil(t, tool.Handler)

		// Verify it's marked as read-only
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "apply_id")
	})
}
