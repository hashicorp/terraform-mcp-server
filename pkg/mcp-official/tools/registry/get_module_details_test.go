// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModuleDetailsTool(t *testing.T) {
	tool := GetModuleDetailsTool()

	assert.Equal(t, "get_module_details", tool.Name)
	assert.Equal(t, "Retrieve documentation for a specific Terraform module", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	assert.Nil(t, tool.InputSchema, "get_module_details relies on the SDK deriving its input schema")
}

func TestGetModuleDetailsParameterValidation(t *testing.T) {
	schema, err := jsonschema.For[GetModuleDetailsArguments](nil)
	require.NoError(t, err)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "module ID present", params: map[string]any{"module_id": "terraform-aws-modules/vpc/aws/6.0.0"}},
		{name: "module ID missing", params: map[string]any{}, expectError: true},
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
