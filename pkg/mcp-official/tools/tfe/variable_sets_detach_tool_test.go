// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetachVariableSetFromWorkspacesTool(t *testing.T) {
	tool := DetachVariableSetFromWorkspacesTool()

	assert.Equal(t, "detach_variable_set_from_workspaces", tool.Name)
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestDetachVariableSetFromWorkspacesArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[DetachVariableSetFromWorkspacesArguments](nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"variable_set_id", "workspace_ids"}, schema.Required)
}

func TestDetachVariableSetFromWorkspacesFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		input   DetachVariableSetFromWorkspacesArguments
		wantErr string
	}{
		{input: DetachVariableSetFromWorkspacesArguments{WorkspaceIDs: "ws-1"}, wantErr: "variable_set_id must not be blank"},
		{input: DetachVariableSetFromWorkspacesArguments{VariableSetID: "varset-1", WorkspaceIDs: "  "}, wantErr: "workspace_ids must not be blank"},
		{input: DetachVariableSetFromWorkspacesArguments{VariableSetID: "varset-1", WorkspaceIDs: ", ,"}, wantErr: "workspace_ids must contain at least one workspace ID"},
	}

	for _, test := range tests {
		_, _, err := DetachVariableSetFromWorkspacesFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}
