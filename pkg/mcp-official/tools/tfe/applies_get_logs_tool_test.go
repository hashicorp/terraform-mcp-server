// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetApplyLogsTool(t *testing.T) {
	tool := GetApplyLogsTool()

	assert.Equal(t, "get_apply_logs", tool.Name)
	assert.Contains(t, tool.Description, "Retrieves the logs of a specific Terraform apply")
	assert.Nil(t, tool.InputSchema, "get_apply_logs relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get logs for a Terraform apply", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetApplyLogsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetApplyLogsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"apply_id"}, schema.Required)

	applyID := schema.Properties["apply_id"]
	require.NotNil(t, applyID)
	assert.Equal(t, "string", applyID.Type)
	assert.Contains(t, applyID.Description, "The ID of the apply to get logs for")
}

func TestGetApplyLogsFunc_RequiresApplyID(t *testing.T) {
	tests := []struct {
		name    string
		applyID string
	}{
		{name: "empty", applyID: ""},
		{name: "whitespace only", applyID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetApplyLogsFunc(t.Context(), nil, GetApplyLogsArguments{ApplyID: tt.applyID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: apply_id")
		})
	}
}
