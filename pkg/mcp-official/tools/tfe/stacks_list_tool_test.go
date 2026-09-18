// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStacksTool(t *testing.T) {
	tool := ListStacksTool()

	assert.Equal(t, "list_stacks", tool.Name)
	assert.Contains(t, tool.Description, "List Terraform Stacks")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List Terraform Stacks in an organization", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListStacksToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListStacksTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "project_id", "search_query", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)

	for _, name := range schema.PropertyOrder {
		require.Contains(t, schema.Properties, name)
	}
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListStacksFunc_RequiresOrgName(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
	}{
		{name: "empty", orgName: ""},
		{name: "whitespace only", orgName: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ListStacksFunc(t.Context(), nil, ListStacksArguments{TerraformOrgName: tt.orgName})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "terraform_org_name must not be blank")
		})
	}
}
