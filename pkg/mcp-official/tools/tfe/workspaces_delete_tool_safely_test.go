// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteWorkspaceSafelyTool(t *testing.T) {
	tool := DeleteWorkspaceSafelyTool()
	assert.Equal(t, "delete_workspace_safely", tool.Name)
	assert.Contains(t, tool.Description, "Safely deletes a Terraform workspace")
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestDeleteWorkspaceSafelyArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[DeleteWorkspaceSafelyArguments](nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"workspace_id"}, schema.Required)
	assert.Equal(t, "string", schema.Properties["workspace_id"].Type)
}

func TestDeleteWorkspaceSafelyFuncRequiresWorkspaceID(t *testing.T) {
	for _, workspaceID := range []string{"", "  "} {
		_, _, err := DeleteWorkspaceSafelyFunc(t.Context(), nil, DeleteWorkspaceSafelyArguments{WorkspaceID: workspaceID})
		require.EqualError(t, err, "workspace_id must not be blank")
	}
}
