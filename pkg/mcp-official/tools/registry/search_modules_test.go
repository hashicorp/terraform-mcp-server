// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
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
		{name: "negative offset", params: map[string]any{"module_query": "aws", "current_offset": -1}, expectError: true},
		{name: "unknown parameter", params: map[string]any{"module_query": "aws", "unknown": true}, expectError: true},
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

func TestUnmarshalTerraformModules_ValidModules(t *testing.T) {
	resp := []byte(`{
		"meta": {
			"limit": 15,
			"current_offset": 0
		},
		"modules": [
			{
				"id": "hashicorp/consul/aws/0.1.0",
				"owner": "hashicorp",
				"namespace": "hashicorp",
				"name": "consul",
				"version": "0.1.0",
				"provider": "aws",
				"description": "Consul module",
				"source": "https://github.com/hashicorp/terraform-aws-consul",
				"tag": "0.1.0",
				"published_at": "2023-01-01T00:00:00Z",
				"downloads": 50,
				"verified": true
			},
			{
				"id": "terraform-aws-modules/vpc/aws/3.14.0",
				"owner": "terraform-aws-modules",
				"namespace": "terraform-aws-modules",
				"name": "vpc",
				"version": "3.14.0",
				"provider": "aws",
				"description": "VPC module",
				"source": "https://github.com/terraform-aws-modules/terraform-aws-vpc",
				"tag": "v3.14.0",
				"published_at": "2023-01-02T00:00:00Z",
				"downloads": 200,
				"verified": true
			}
		]
	}`)

	out, err := unmarshalTerraformModules(resp, "aws")
	require.NoError(t, err)
	assert.Contains(t, out, "Available Terraform Modules (top matches) for aws")
	assert.Contains(t, out, "terraform-aws-modules/vpc/aws/3.14.0")
	assert.Contains(t, out, "hashicorp/consul/aws/0.1.0")

	// Check that higher download module appears first (vpc 200 before consul 50)
	vpcIndex := strings.Index(out, "terraform-aws-modules/vpc/aws/3.14.0")
	consulIndex := strings.Index(out, "hashicorp/consul/aws/0.1.0")
	require.NotEqual(t, -1, vpcIndex)
	require.NotEqual(t, -1, consulIndex)
	assert.Less(t, vpcIndex, consulIndex)
}

func TestUnmarshalTerraformModules_EmptyModules(t *testing.T) {
	resp := []byte(`{
		"meta": {
			"limit": 15,
			"current_offset": 0
		},
		"modules": []
	}`)

	out, err := unmarshalTerraformModules(resp, "unknown")
	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestUnmarshalTerraformModules_EmptyObject(t *testing.T) {
	resp := []byte(`{}`)

	out, err := unmarshalTerraformModules(resp, "unknown")
	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestUnmarshalTerraformModules_InvalidJSON(t *testing.T) {
	resp := []byte(`not a json`)

	_, err := unmarshalTerraformModules(resp, "unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshalling modules")
}
