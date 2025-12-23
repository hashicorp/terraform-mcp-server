// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetRunLogs(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GetRunLogs(logger)

		assert.Equal(t, "get_run_logs", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Fetches logs from a Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that read-only hint is true
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check that destructive hint is false
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "run_id")

		// Check that log_type property exists
		logTypeProperty := tool.Tool.InputSchema.Properties["log_type"]
		assert.NotNil(t, logTypeProperty)

		// Check that include_metadata property exists
		includeMetadataProperty := tool.Tool.InputSchema.Properties["include_metadata"]
		assert.NotNil(t, includeMetadataProperty)
	})
}

