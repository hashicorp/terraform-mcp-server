// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateRunSafe(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRunSafe(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is false
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)
	})
}

func TestCreateRun(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRun(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is true
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)
	})
}
