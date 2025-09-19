// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCreateWorkspaceTags(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateWorkspaceTags(logger)

		assert.Equal(t, "create_workspace_tags", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Add tags to a Terraform workspace")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "tags")
	})
}

func TestReadWorkspaceTags(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := ReadWorkspaceTags(logger)

		assert.Equal(t, "read_workspace_tags", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Read all tags from a Terraform workspace")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
	})
}
