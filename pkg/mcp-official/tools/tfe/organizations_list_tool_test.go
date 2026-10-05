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

func TestListTerraformOrganizationsTool(t *testing.T) {
	tool := ListTerraformOrganizationsTool()

	assert.Equal(t, "list_terraform_orgs", tool.Name)
	assert.Contains(t, tool.Description, "Fetches a list of all Terraform organizations")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List all Terraform organizations", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
}

func TestListTerraformOrganizationsToolOutputSchema(t *testing.T) {
	schema, ok := ListTerraformOrganizationsTool().OutputSchema.(*jsonschema.Schema)
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

	results := []OrganizationSummaryList{
		{Items: make([]OrganizationSummary, 0)},
		{Items: []OrganizationSummary{{Name: "org", Email: "org@example.com", CreatedAt: time.Now()}}},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestListTerraformOrganizationsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListTerraformOrganizationsTool().InputSchema)

	assert.Equal(t, []string{"page", "pageSize"}, schema.PropertyOrder)
	assert.Empty(t, schema.Required)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}
