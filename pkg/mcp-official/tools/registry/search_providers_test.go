// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchProvidersTool(t *testing.T) {
	tool := SearchProvidersTool()

	assert.Equal(t, "search_providers", tool.Name)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Identify the most relevant provider document ID for a Terraform service", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "provider_name")
	assert.NotContains(t, schema.Required, "provider_namespace")
	assert.Contains(t, schema.Required, "service_slug")
	assert.NotContains(t, schema.Required, "provider_document_type")
	assert.NotContains(t, schema.Required, "provider_version")

	providerNamespaceSchema := schema.Properties["provider_namespace"]
	require.NotNil(t, providerNamespaceSchema)
	assert.JSONEq(t, `"hashicorp"`, string(providerNamespaceSchema.Default))

	providerDocumentTypeSchema := schema.Properties["provider_document_type"]
	require.NotNil(t, providerDocumentTypeSchema)
	assert.Equal(t, []any{"resources", "data-sources", "functions", "guides", "overview", "actions", "list-resources"}, providerDocumentTypeSchema.Enum)
	assert.JSONEq(t, `"resources"`, string(providerDocumentTypeSchema.Default))

	providerVersionSchema := schema.Properties["provider_version"]
	require.NotNil(t, providerVersionSchema)
	assert.JSONEq(t, `"latest"`, string(providerVersionSchema.Default))
}

func TestSearchProvidersParameterValidation(t *testing.T) {
	tool := SearchProvidersTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "required parameters present", params: map[string]any{"provider_name": "aws", "service_slug": "s3"}},
		{name: "all parameters present", params: map[string]any{"provider_name": "aws", "provider_namespace": "hashicorp", "service_slug": "s3", "provider_document_type": "resources", "provider_version": "5.0.0"}},
		{name: "provider name missing", params: map[string]any{"service_slug": "s3"}, expectError: true},
		{name: "service slug missing", params: map[string]any{"provider_name": "aws"}, expectError: true},
		{name: "invalid document type", params: map[string]any{"provider_name": "aws", "service_slug": "s3", "provider_document_type": "invalid"}, expectError: true},
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
