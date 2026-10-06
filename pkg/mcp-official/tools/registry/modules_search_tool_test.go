// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchModulesTool(t *testing.T) {
	tool := SearchModulesTool()

	assert.Equal(t, "search_modules", tool.Name)
	assert.Equal(t, "Search and match Terraform modules based on name and relevance", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "module_query")
	assert.NotContains(t, schema.Required, "current_offset")

	currentOffsetSchema := schema.Properties["current_offset"]
	assert.JSONEq(t, `0`, string(currentOffsetSchema.Default))
}

func TestSearchModulesParameterValidation(t *testing.T) {
	tool := SearchModulesTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "query present", params: map[string]any{"module_query": "aws"}},
		{name: "empty query present", params: map[string]any{"module_query": ""}},
		{name: "query and offset present", params: map[string]any{"module_query": "aws", "current_offset": 15}},
		{name: "query missing", params: map[string]any{}, expectError: true},
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
