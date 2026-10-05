// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateVariableInVariableSetTool(t *testing.T) {
	tool := CreateVariableInVariableSetTool()

	assert.Equal(t, "create_variable_in_variable_set", tool.Name)
	assert.Contains(t, tool.Description, "Create a new variable in a variable set")
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateVariableInVariableSetToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateVariableInVariableSetTool().InputSchema)

	assert.Equal(t, []string{"variable_set_id", "key", "value", "description", "category", "hcl", "sensitive"}, schema.PropertyOrder)
	assert.ElementsMatch(t, []string{"variable_set_id", "key", "value"}, schema.Required)
	assert.Equal(t, []any{"terraform", "env"}, schema.Properties["category"].Enum)
	assert.JSONEq(t, `"terraform"`, string(schema.Properties["category"].Default))
	assert.JSONEq(t, "false", string(schema.Properties["hcl"].Default))
	assert.JSONEq(t, "false", string(schema.Properties["sensitive"].Default))
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateVariableInVariableSetFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateVariableInVariableSetArguments
		wantErr string
	}{
		{name: "blank variable set ID", input: CreateVariableInVariableSetArguments{Key: "key"}, wantErr: "variable_set_id must not be blank"},
		{name: "blank key", input: CreateVariableInVariableSetArguments{VariableSetID: "varset-1", Key: "  "}, wantErr: "key must not be blank"},
		{name: "invalid category", input: CreateVariableInVariableSetArguments{VariableSetID: "varset-1", Key: "key", Category: "invalid"}, wantErr: "category must be one of: terraform, env"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := CreateVariableInVariableSetFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
		})
	}
}
