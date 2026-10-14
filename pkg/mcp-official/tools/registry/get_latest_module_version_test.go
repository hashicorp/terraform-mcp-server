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
	assert.Nil(t, tool.InputSchema, "get_latest_module_version relies on the SDK deriving its input schema")
}

func TestGetLatestModuleVersionParameterValidation(t *testing.T) {
	schema, err := jsonschema.For[GetLatestModuleVersionArguments](nil)
	require.NoError(t, err)
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
