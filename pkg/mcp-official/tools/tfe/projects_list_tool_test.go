// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListProjectsTool(t *testing.T) {
	tool := ListProjectsTool()

	assert.Equal(t, "list_terraform_projects", tool.Name)
	assert.Contains(t, tool.Description, "Search and list Terraform projects")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List all Terraform projects", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListProjectsToolOutputSchema(t *testing.T) {
	schema, ok := ListProjectsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "object", schema.Type)

	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Empty(t, items.Types)
	require.NotNil(t, items.Items)
	assert.Equal(t, "object", items.Items.Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []ProjectSummaryList{
		{Items: make([]ProjectSummary, 0)},
		{Items: []ProjectSummary{{ID: "prj-123", Name: "project"}}},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestListProjectsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListProjectsTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)

	orgName := schema.Properties["terraform_org_name"]
	require.NotNil(t, orgName)
	assert.Equal(t, "string", orgName.Type)
	assert.Contains(t, orgName.Description, "The name of the Terraform Cloud/Enterprise organization")

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListProjectsFunc_RequiresOrgName(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
	}{
		{name: "empty", orgName: ""},
		{name: "whitespace only", orgName: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ListProjectsFunc(t.Context(), nil, ListProjectsArguments{TerraformOrgName: tt.orgName})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "terraform_org_name must not be blank")
		})
	}
}
