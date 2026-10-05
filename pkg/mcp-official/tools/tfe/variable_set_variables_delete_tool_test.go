// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteVariableInVariableSetTool(t *testing.T) {
	tool := DeleteVariableInVariableSetTool()

	assert.Equal(t, "delete_variable_in_variable_set", tool.Name)
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestDeleteVariableInVariableSetArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[DeleteVariableInVariableSetArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.ElementsMatch(t, []string{"variable_set_id", "variable_id"}, schema.Required)
	assert.Equal(t, "string", schema.Properties["variable_set_id"].Type)
	assert.Equal(t, "string", schema.Properties["variable_id"].Type)
}

func TestDeleteVariableInVariableSetFuncRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		input   DeleteVariableInVariableSetArguments
		wantErr string
	}{
		{input: DeleteVariableInVariableSetArguments{VariableID: "var-1"}, wantErr: "variable_set_id must not be blank"},
		{input: DeleteVariableInVariableSetArguments{VariableSetID: "varset-1", VariableID: "  "}, wantErr: "variable_id must not be blank"},
	}

	for _, test := range tests {
		_, _, err := DeleteVariableInVariableSetFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}
