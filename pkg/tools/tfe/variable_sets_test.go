// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestListVariableSets(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := ListVariableSets(logger)

		assert.Equal(t, "list_variable_sets", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "List all variable sets in an organization")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
	})
}

func TestCreateVariableSet(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateVariableSet(logger)

		assert.Equal(t, "create_variable_set", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Create a new variable set")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "name")
	})
}

func TestCreateVariableInVariableSet(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateVariableInVariableSet(logger)

		assert.Equal(t, "create_variable_in_variable_set", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Create a new variable in a variable set")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "variable_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "key")
		assert.Contains(t, tool.Tool.InputSchema.Required, "value")
	})
}

func TestDeleteVariableFromVariableSet(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := DeleteVariableInVariableSet(logger)

		assert.Equal(t, "delete_variable_in_variable_set", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Delete a variable in a variable set")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "variable_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "variable_id")
	})
}

func TestAttachVariableSetToWorkspaces(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := AttachVariableSetToWorkspaces(logger)

		assert.Equal(t, "attach_variable_set_to_workspaces", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Attach a variable set to one or more workspaces")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "variable_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_ids")
	})
}

func TestDetachVariableSetFromWorkspaces(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := DetachVariableSetFromWorkspaces(logger)

		assert.Equal(t, "detach_variable_set_from_workspaces", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Detach a variable set from one or more workspaces")
		assert.NotNil(t, tool.Handler)

		assert.Contains(t, tool.Tool.InputSchema.Required, "variable_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_ids")
	})
}
