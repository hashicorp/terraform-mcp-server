// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorkspacesTool(t *testing.T) {
	tool := ListWorkspacesTool()

	assert.Equal(t, "list_workspaces", tool.Name)
	assert.Contains(t, tool.Description, "Search and list Terraform workspaces")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List Terraform workspaces with queries", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListWorkspacesToolOutputSchema(t *testing.T) {
	schema, ok := ListWorkspacesTool().OutputSchema.(*jsonschema.Schema)
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

	results := []WorkspaceSummaryList{
		{Items: make([]WorkspaceSummary, 0)},
		{Items: []WorkspaceSummary{{ID: "ws-123", Name: "workspace", CreatedAt: time.Now()}}},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestListWorkspacesToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListWorkspacesTool().InputSchema)

	assert.Equal(t, []string{
		"terraform_org_name",
		"project_id",
		"search_query",
		"tags",
		"exclude_tags",
		"wildcard_name",
		"page",
		"pageSize",
	}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)

	for _, name := range schema.PropertyOrder {
		require.Contains(t, schema.Properties, name)
	}

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListWorkspacesFunc_RequiresOrgName(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
	}{
		{name: "empty", orgName: ""},
		{name: "whitespace only", orgName: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ListWorkspacesFunc(t.Context(), nil, ListWorkspacesArguments{TerraformOrgName: tt.orgName})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "terraform_org_name must not be blank")
		})
	}
}

func TestNormaliseCommaSeparated(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "whitespace only", value: "   ", want: ""},
		{name: "single value", value: "prod", want: "prod"},
		{name: "trims around each element", value: "a, b , c", want: "a,b,c"},
		{name: "drops empty segments", value: "a, , c", want: "a,c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normaliseCommaSeparated(tt.value))
		})
	}
}
