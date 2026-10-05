// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTeamsTool(t *testing.T) {
	tool := ListTeamsTool()

	assert.Equal(t, "list_teams", tool.Name)
	assert.Contains(t, tool.Description, "Search and list teams")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List teams in a Terraform organization", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListTeamsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListTeamsTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "team_names", "search_query", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)

	for _, name := range schema.PropertyOrder {
		require.Contains(t, schema.Properties, name)
	}
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListTeamsToolOutputSchema(t *testing.T) {
	schema := inputSchema(t, ListTeamsTool().OutputSchema)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"items"}, schema.Required)

	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	require.NotNil(t, items.Items)
	assert.Equal(t, "object", items.Items.Type)
	assert.Equal(t, []string{"id", "name", "visibility", "users-count"}, items.Items.Required)
}

func TestListTeamsFunc_RequiresOrgName(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
	}{
		{name: "empty", orgName: ""},
		{name: "whitespace only", orgName: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ListTeamsFunc(t.Context(), nil, ListTeamsArguments{TerraformOrgName: tt.orgName})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "terraform_org_name must not be blank")
		})
	}
}
