// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVariableSetsTool(t *testing.T) {
	tool := ListVariableSetsTool()

	assert.Equal(t, "list_variable_sets", tool.Name)
	assert.Contains(t, tool.Description, "List variable sets in an organization")
	assert.Nil(t, tool.OutputSchema)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List Terraform variable sets", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListVariableSetsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListVariableSetsTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "query", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)
	assert.Equal(t, "string", schema.Properties["terraform_org_name"].Type)
	assert.Equal(t, "string", schema.Properties["query"].Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListVariableSetsFuncRequiresOrganization(t *testing.T) {
	for _, orgName := range []string{"", "   "} {
		_, _, err := ListVariableSetsFunc(t.Context(), nil, ListVariableSetsArguments{TerraformOrgName: orgName})
		require.EqualError(t, err, "terraform_org_name must not be blank")
	}
}
