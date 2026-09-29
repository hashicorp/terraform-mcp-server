// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorkspaceVariablesTool(t *testing.T) {
	tool := ListWorkspaceVariablesTool()

	assert.Equal(t, "list_workspace_variables", tool.Name)
	assert.Contains(t, tool.Description, "Sensitive variable values are not returned")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "List variables in a Terraform workspace", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListWorkspaceVariablesToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListWorkspaceVariablesTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "workspace_name", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)

	for _, name := range schema.PropertyOrder {
		require.Contains(t, schema.Properties, name)
	}
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListWorkspaceVariablesFunc_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   ListWorkspaceVariablesArguments
		wantErr string
	}{
		{name: "blank org", input: ListWorkspaceVariablesArguments{TerraformOrgName: " ", WorkspaceName: "ws"}, wantErr: "terraform_org_name must not be blank"},
		{name: "blank workspace", input: ListWorkspaceVariablesArguments{TerraformOrgName: "org", WorkspaceName: " "}, wantErr: "workspace_name must not be blank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ListWorkspaceVariablesFunc(t.Context(), nil, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestVariableToSummary(t *testing.T) {
	t.Run("maps a plain variable", func(t *testing.T) {
		s := variableToSummary(&tfe.Variable{
			ID:          "var-abc123",
			Key:         "region",
			Value:       "us-east-1",
			Description: "deploy region",
			Category:    tfe.CategoryTerraform,
			HCL:         false,
			Sensitive:   false,
		})

		assert.Equal(t, "var-abc123", s.ID)
		assert.Equal(t, "region", s.Key)
		assert.Equal(t, "us-east-1", s.Value)
		assert.Equal(t, "deploy region", s.Description)
		assert.Equal(t, "terraform", s.Category)
		assert.False(t, s.HCL)
		assert.False(t, s.Sensitive)
	})

	t.Run("omits value for sensitive variables", func(t *testing.T) {
		s := variableToSummary(&tfe.Variable{
			ID:        "var-abc123",
			Key:       "api_token",
			Value:     "should-not-appear",
			Category:  tfe.CategoryEnv,
			Sensitive: true,
		})

		assert.Empty(t, s.Value)
		assert.True(t, s.Sensitive)
		assert.Equal(t, "env", s.Category)
	})

	t.Run("preserves hcl flag", func(t *testing.T) {
		s := variableToSummary(&tfe.Variable{
			ID:       "var-abc123",
			Key:      "tags",
			Value:    `{ env = "prod" }`,
			Category: tfe.CategoryTerraform,
			HCL:      true,
		})

		assert.True(t, s.HCL)
		assert.Equal(t, `{ env = "prod" }`, s.Value)
	})
}
