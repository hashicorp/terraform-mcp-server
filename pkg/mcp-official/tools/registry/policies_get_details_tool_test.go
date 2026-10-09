// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPolicyDetailsTool(t *testing.T) {
	tool := GetPolicyDetailsTool()

	assert.Equal(t, "get_policy_details", tool.Name)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Fetch detailed Terraform policy documentation using a terraform_policy_id", tool.Annotations.Title)

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "terraform_policy_id")
}

func TestGetPolicyDetailsParameterValidation(t *testing.T) {
	tool := GetPolicyDetailsTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "policy ID present", params: map[string]any{"terraform_policy_id": "policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1"}},
		{name: "policy ID missing", params: map[string]any{}, expectError: true},
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
