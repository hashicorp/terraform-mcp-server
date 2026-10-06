// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestModuleVersionTool(t *testing.T) {
	tool := GetLatestModuleVersionTool()

	assert.Equal(t, "get_latest_module_version", tool.Name)
	assert.Equal(t, "Get Latest Module Version", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, _ := tool.InputSchema.(*jsonschema.Schema)
	assert.Contains(t, schema.Required, "module_publisher")
	assert.Contains(t, schema.Required, "module_name")
	assert.Contains(t, schema.Required, "module_provider")
}

func TestGetLatestModuleVersionParameterValidation(t *testing.T) {
	tool := GetLatestModuleVersionTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "all parameters present", params: map[string]any{"module_publisher": "terraform-aws-modules", "module_name": "vpc", "module_provider": "aws"}},
		{name: "publisher missing", params: map[string]any{"module_name": "vpc", "module_provider": "aws"}, expectError: true},
		{name: "name missing", params: map[string]any{"module_publisher": "terraform-aws-modules", "module_provider": "aws"}, expectError: true},
		{name: "provider missing", params: map[string]any{"module_publisher": "terraform-aws-modules", "module_name": "vpc"}, expectError: true},
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
