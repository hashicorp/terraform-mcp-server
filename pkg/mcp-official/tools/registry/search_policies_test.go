// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchPoliciesTool(t *testing.T) {
	tool := SearchPoliciesTool()

	assert.Equal(t, "search_policies", tool.Name)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Search and match Terraform policies based on name and relevance", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	assert.Nil(t, tool.InputSchema, "search_policies relies on the SDK deriving its input schema")
}

func TestSearchPoliciesParameterValidation(t *testing.T) {
	schema, err := jsonschema.For[SearchPoliciesArguments](nil)
	require.NoError(t, err)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "policy query present", params: map[string]any{"policy_query": "aws"}},
		{name: "policy query missing", params: map[string]any{}, expectError: true},
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
