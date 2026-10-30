// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetPlanChanges(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	tool := GetPlanChanges(logger)

	assert.Equal(t, "get_plan_changes", tool.Tool.Name)
	assert.Contains(t, tool.Tool.Description, "Never returns attribute values")
	assert.NotNil(t, tool.Handler)

	assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)

	assert.Empty(t, tool.Tool.InputSchema.Required)
	assert.Contains(t, tool.Tool.InputSchema.Properties, "run_id")
	assert.Contains(t, tool.Tool.InputSchema.Properties, "plan_id")
}
