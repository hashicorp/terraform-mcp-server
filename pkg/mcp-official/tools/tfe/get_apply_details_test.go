// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetApplyDetailsTool(t *testing.T) {
	tool := GetApplyDetailsTool()

	assert.Equal(t, "get_apply_details", tool.Name)
	assert.Contains(t, tool.Description, "Fetches detailed information about a specific Terraform apply")
	assert.Nil(t, tool.InputSchema, "get_apply_details relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get detailed information about a Terraform apply", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetApplyDetailsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetApplyDetailsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"apply_id"}, schema.Required)

	applyID := schema.Properties["apply_id"]
	require.NotNil(t, applyID)
	assert.Equal(t, "string", applyID.Type)
	assert.Contains(t, applyID.Description, "The ID of the apply to get details for")
}

func TestGetApplyDetailsFunc_RequiresApplyID(t *testing.T) {
	tests := []struct {
		name    string
		applyID string
	}{
		{name: "empty", applyID: ""},
		{name: "whitespace only", applyID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetApplyDetailsFunc(t.Context(), nil, GetApplyDetailsArguments{ApplyID: tt.applyID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: apply_id")
		})
	}
}
