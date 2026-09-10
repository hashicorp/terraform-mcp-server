// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

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

func TestListTerraformOrganizationsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListTerraformOrganizationsTool().InputSchema)

	assert.Equal(t, []string{"page", "pageSize"}, schema.PropertyOrder)
	assert.Empty(t, schema.Required)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}
