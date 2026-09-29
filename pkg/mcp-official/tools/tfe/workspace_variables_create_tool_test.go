// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWorkspaceVariableTool(t *testing.T) {
	tool := CreateWorkspaceVariableTool()

	assert.Equal(t, "create_workspace_variable", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new variable")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Create a variable in a Terraform workspace", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateWorkspaceVariableToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateWorkspaceVariableTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "workspace_name", "variable_key", "variable_value", "category", "description", "hcl", "sensitive"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name", "variable_key", "variable_value", "category"}, schema.Required)

	category := schema.Properties["category"]
	require.NotNil(t, category)
	assert.Equal(t, []any{"terraform", "env"}, category.Enum)

	require.Contains(t, schema.Properties, "hcl")
	require.Contains(t, schema.Properties, "sensitive")
	assert.NotContains(t, schema.Required, "hcl")
	assert.NotContains(t, schema.Required, "sensitive")

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateWorkspaceVariableFunc_Validation(t *testing.T) {
	valid := CreateWorkspaceVariableArguments{
		TerraformOrgName: "org",
		WorkspaceName:    "ws",
		Key:              "region",
		Value:            "us-east-1",
		Category:         "terraform",
	}

	tests := []struct {
		name    string
		mutate  func(*CreateWorkspaceVariableArguments)
		wantErr string
	}{
		{name: "blank org", mutate: func(a *CreateWorkspaceVariableArguments) { a.TerraformOrgName = " " }, wantErr: "terraform_org_name must not be blank"},
		{name: "blank workspace", mutate: func(a *CreateWorkspaceVariableArguments) { a.WorkspaceName = " " }, wantErr: "workspace_name must not be blank"},
		{name: "blank key", mutate: func(a *CreateWorkspaceVariableArguments) { a.Key = " " }, wantErr: "variable_key must not be blank"},
		{name: "bad category", mutate: func(a *CreateWorkspaceVariableArguments) { a.Category = "secret" }, wantErr: "invalid category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)
			_, _, err := CreateWorkspaceVariableFunc(t.Context(), nil, input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
