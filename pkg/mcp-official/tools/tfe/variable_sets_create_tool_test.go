// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateVariableSetTool(t *testing.T) {
	tool := CreateVariableSetTool()

	assert.Equal(t, "create_variable_set", tool.Name)
	assert.Contains(t, tool.Description, "Create a new variable set")
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateVariableSetToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateVariableSetTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "name", "description", "global"}, schema.PropertyOrder)
	assert.ElementsMatch(t, []string{"terraform_org_name", "name"}, schema.Required)
	assert.JSONEq(t, "false", string(schema.Properties["global"].Default))
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateVariableSetFuncRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateVariableSetArguments
		wantErr string
	}{
		{name: "blank organization", input: CreateVariableSetArguments{Name: "set"}, wantErr: "terraform_org_name must not be blank"},
		{name: "blank name", input: CreateVariableSetArguments{TerraformOrgName: "org", Name: "  "}, wantErr: "name must not be blank"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := CreateVariableSetFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
		})
	}
}
