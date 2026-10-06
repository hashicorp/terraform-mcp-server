// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForceUnlockWorkspaceArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[ForceUnlockWorkspaceArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"workspace_id"}, schema.Required)

	workspaceID := schema.Properties["workspace_id"]
	require.NotNil(t, workspaceID)
	assert.Equal(t, "string", workspaceID.Type)
	assert.Contains(t, workspaceID.Description, "The ID of the workspace to force unlock")
}

func TestForceUnlockWorkspaceFunc_RequiresWorkspaceID(t *testing.T) {
	tests := []struct {
		name        string
		workspaceID string
	}{
		{name: "empty", workspaceID: ""},
		{name: "whitespace only", workspaceID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ForceUnlockWorkspaceFunc(t.Context(), nil, ForceUnlockWorkspaceArguments{WorkspaceID: tt.workspaceID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "workspace_id must not be blank")
		})
	}
}
