// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStateVersionsTool(t *testing.T) {
	tool := ListStateVersionsTool()

	assert.Equal(t, "list_state_versions", tool.Name)
	assert.Contains(t, tool.Annotations.Title, "List Terraform state versions")

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "terraform_org_name")
	assert.Contains(t, schema.Required, "workspace_name")
	assert.NotContains(t, schema.Required, "page")
	assert.NotContains(t, schema.Required, "pageSize")
}

func TestListStateVersionsParameterValidation(t *testing.T) {
	tool := ListStateVersionsTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "both params present", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace"}},
		{name: "missing organization", params: map[string]any{"workspace_name": "my-workspace"}, expectError: true},
		{name: "missing workspace", params: map[string]any{"terraform_org_name": "my-org"}, expectError: true},
		{name: "both params missing", params: map[string]any{}, expectError: true},
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
