// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVariableSetsTool(t *testing.T) {
	tool := ListVariableSetsTool()

	assert.Equal(t, "list_variable_sets", tool.Name)
	assert.Contains(t, tool.Description, "List variable sets in an organization")
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List Terraform variable sets", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListVariableSetsToolOutputSchema(t *testing.T) {
	schema, ok := ListVariableSetsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"items"}, schema.Required)
	assert.Equal(t, []string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"}, schema.PropertyOrder)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Empty(t, items.Types)
	require.NotNil(t, items.Items)
	assert.Equal(t, "object", items.Items.Type)
	assert.Equal(t, []string{"variable_set_id", "variable_set_name", "description", "global", "priority"}, items.Items.PropertyOrder)
	assert.Equal(t, items.Items.PropertyOrder, items.Items.Required)
	require.NotNil(t, items.Items.AdditionalProperties)
	assert.NotNil(t, items.Items.AdditionalProperties.Not)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []VariableSetSummaryList{
		{Items: make([]VariableSetSummary, 0)},
		{
			Items: []VariableSetSummary{{
				ID:          "varset-123",
				Name:        "shared variables",
				Description: "Shared across workspaces",
				Global:      true,
				Priority:    false,
			}},
			PaginationDetails: PaginationDetails{CurrentPage: 1, TotalCount: 1, TotalPages: 1},
		},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
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
