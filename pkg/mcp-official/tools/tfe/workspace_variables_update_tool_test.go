// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkspaceVariableTool(t *testing.T) {
	tool := UpdateWorkspaceVariableTool()

	assert.Equal(t, "update_workspace_variable", tool.Name)
	assert.Contains(t, tool.Description, "Only the fields you provide are changed")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Update a variable in a Terraform workspace", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestUpdateWorkspaceVariableToolInputSchema(t *testing.T) {
	schema := inputSchema(t, UpdateWorkspaceVariableTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "workspace_name", "variable_id"}, schema.Required)

	category := schema.Properties["category"]
	require.NotNil(t, category)
	assert.Equal(t, []any{"terraform", "env"}, category.Enum)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestUpdateWorkspaceVariableFunc_Validation(t *testing.T) {
	str := func(s string) *string { return &s }

	tests := []struct {
		name    string
		input   UpdateWorkspaceVariableArguments
		wantErr string
	}{
		{name: "blank org", input: UpdateWorkspaceVariableArguments{TerraformOrgName: " ", WorkspaceName: "ws", VariableID: "var-1", Value: str("x")}, wantErr: "terraform_org_name must not be blank"},
		{name: "blank workspace", input: UpdateWorkspaceVariableArguments{TerraformOrgName: "org", WorkspaceName: " ", VariableID: "var-1", Value: str("x")}, wantErr: "workspace_name must not be blank"},
		{name: "blank variable id", input: UpdateWorkspaceVariableArguments{TerraformOrgName: "org", WorkspaceName: "ws", VariableID: " ", Value: str("x")}, wantErr: "variable_id must not be blank"},
		{name: "nothing to update", input: UpdateWorkspaceVariableArguments{TerraformOrgName: "org", WorkspaceName: "ws", VariableID: "var-1"}, wantErr: "at least one of"},
		{name: "bad category", input: UpdateWorkspaceVariableArguments{TerraformOrgName: "org", WorkspaceName: "ws", VariableID: "var-1", Category: "secret"}, wantErr: "invalid category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := UpdateWorkspaceVariableFunc(t.Context(), nil, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestUpdateWorkspaceVariableFunc_EmptyValueIsAChange(t *testing.T) {
	empty := ""
	_, _, err := UpdateWorkspaceVariableFunc(t.Context(), nil, UpdateWorkspaceVariableArguments{
		TerraformOrgName: "org",
		WorkspaceName:    "ws",
		VariableID:       "var-1",
		Value:            &empty,
	})
	// gets past validation and fails on the client, which is the point.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting Terraform client")
}

func TestUpdateWorkspaceVariableToolOutputSchema(t *testing.T) {
	schema := inputSchema(t, UpdateWorkspaceVariableTool().OutputSchema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Required, "id")
	assert.NotContains(t, schema.Required, "value")
}
