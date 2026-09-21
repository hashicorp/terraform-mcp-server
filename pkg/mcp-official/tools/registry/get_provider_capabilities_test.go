// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
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

func TestCapabilitiesExampleLimit(t *testing.T) {
	for _, count := range []int{1, 10, 11} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			docs := registryapi.ProviderDocs{}
			for i := 0; i < count; i++ {
				docs.Docs = append(docs.Docs, registryapi.ProviderDoc{ID: fmt.Sprint(i), Title: fmt.Sprintf("resource_%d", i), Category: "resources", Language: "hcl"})
			}
			result := analyzeAndFormatCapabilities(docs, "hashicorp", "test", "1.0.0")
			require.Contains(t, result, fmt.Sprintf("Resources: %d available", count))
			if count <= 10 {
				require.Contains(t, result, fmt.Sprintf("resource_%d (provider_doc_id: %d)", count-1, count-1))
				require.NotContains(t, result, "... and")
			} else {
				require.Contains(t, result, "resource_2 (provider_doc_id: 2)")
				require.NotContains(t, result, "resource_3 (")
				require.Contains(t, result, "... and 8 more")
			}
		})
	}
}

func TestCapabilitiesCategories(t *testing.T) {
	docs := registryapi.ProviderDocs{}
	for _, category := range []string{"resources", "data-sources", "functions", "guides", "actions", "ephemeral-resources", "list-resources"} {
		docs.Docs = append(docs.Docs, registryapi.ProviderDoc{Category: category, Language: "hcl", Title: category, ID: "123"})
	}
	result := analyzeAndFormatCapabilities(docs, "hashicorp", "test", "1.0.0")
	for _, title := range []string{"Resources", "Data Sources", "Functions", "Guides", "Actions", "Ephemeral Resources", "List Resources"} {
		require.Contains(t, result, title+": 1 available")
	}
}
