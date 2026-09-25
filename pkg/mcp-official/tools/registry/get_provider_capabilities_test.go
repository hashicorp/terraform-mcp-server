// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProviderCapabilitiesTool(t *testing.T) {
	tool := GetProviderCapabilitiesTool()

	assert.Equal(t, "get_provider_capabilities", tool.Name)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get Terraform provider capabilities and supported features", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "namespace")
	assert.Contains(t, schema.Required, "name")

	versionSchema := schema.Properties["version"]
	require.NotNil(t, versionSchema)
	assert.JSONEq(t, `"latest"`, string(versionSchema.Default))
}

func TestGetProviderCapabilitiesParameterValidation(t *testing.T) {
	tool := GetProviderCapabilitiesTool()
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
		{name: "optional version present", params: map[string]any{"namespace": "hashicorp", "name": "aws", "version": "5.0.0"}},
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
