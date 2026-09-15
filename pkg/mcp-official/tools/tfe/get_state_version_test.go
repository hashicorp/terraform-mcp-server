// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStateVersionTool(t *testing.T) {
	tool := GetStateVersionTool()

	assert.Equal(t, "get_state_version", tool.Name)
	assert.Contains(t, tool.Annotations.Title, "Get Terraform state version")

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.NotContains(t, schema.Required, "state_version_id")
	assert.NotContains(t, schema.Required, "workspace_id")
	require.Len(t, schema.AnyOf, 2)
	assert.Contains(t, schema.AnyOf[0].Required, "state_version_id")
	assert.Contains(t, schema.AnyOf[1].Required, "workspace_id")
}

func TestGetStateVersionParameterValidation(t *testing.T) {
	tool := GetStateVersionTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "state version present", params: map[string]any{"state_version_id": "sv-abc123"}},
		{name: "both IDs missing", params: map[string]any{}, expectError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolved.Validate(tt.params)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
