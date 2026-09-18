// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestProviderVersionTool(t *testing.T) {
	tool := GetLatestProviderVersionTool()

	assert.Equal(t, "get_latest_provider_version", tool.Name)
	assert.Equal(t, "Get Latest Provider Version", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "namespace")
	assert.Contains(t, schema.Required, "name")
}

func TestGetLatestProviderVersionParameterValidation(t *testing.T) {
	tool := GetLatestProviderVersionTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "namespace and name present", params: map[string]any{"namespace": "hashicorp", "name": "aws"}},
		{name: "namespace missing", params: map[string]any{"name": "aws"}, expectError: true},
		{name: "name missing", params: map[string]any{"namespace": "hashicorp"}, expectError: true},
		{name: "both parameters missing", params: map[string]any{}, expectError: true},
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
