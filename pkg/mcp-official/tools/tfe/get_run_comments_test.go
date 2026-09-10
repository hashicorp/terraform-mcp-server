// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRunCommentsTool(t *testing.T) {
	tool := GetRunCommentsTool()

	assert.Equal(t, "get_run_comments", tool.Name)
	assert.Contains(t, tool.Annotations.Title, "Get all comments for a given Terraform run.")

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

// get_run_comments leaves InputSchema nil so the SDK derives it from
// GetRunCommentsArguments. Derive it the same way to confirm the struct tags
// still make run_id a required string.
func TestGetRunCommentsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetRunCommentsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"run_id"}, schema.Required)

	runID := schema.Properties["run_id"]
	require.NotNil(t, runID)
	assert.Equal(t, "string", runID.Type)
	assert.Contains(t, runID.Description, "The ID of the Terraform run to retrieve comments for")
}

func TestGetRunCommentsFunc_RequiresRunID(t *testing.T) {
	tests := []struct {
		name  string
		runID string
	}{
		{name: "empty", runID: ""},
		{name: "whitespace only", runID: "   "},
		{name: "hash prefix only", runID: "#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetRunCommentsFunc(t.Context(), nil, GetRunCommentsArguments{RunID: tt.runID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: run_id")
		})
	}
}
