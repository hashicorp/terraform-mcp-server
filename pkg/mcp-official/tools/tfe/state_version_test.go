// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStateVersionsTool(t *testing.T) {
	tool := ListStateVersionsTool()

	assert.Equal(t, "list_state_versions", tool.Name)
	assert.Contains(t, tool.Description, "List all the state versions")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List Terraform state versions", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetStateVersionTool(t *testing.T) {
	tool := GetStateVersionTool()

	assert.Equal(t, "get_state_version", tool.Name)
	assert.Contains(t, tool.Description, "Retrieves a Terraform state version")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get Terraform state version", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}
