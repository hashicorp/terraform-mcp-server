// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAttachPolicySetToWorkspaces(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := AttachPolicySetToWorkspaces(logger)

		assert.Equal(t, "attach_policy_set_to_workspaces", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Attach a policy set")
		assert.NotNil(t, tool.Handler)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "policy_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_ids")
	})
}
