// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRunCommentsTool(t *testing.T) {
	tool := GetRunCommentsTool()
	assert.Equal(t, "get_run_comments", tool.Name)
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetRunCommentsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetRunCommentsArguments](nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"run_id"}, schema.Required)
}

func TestGetRunCommentsFuncRequiresRunID(t *testing.T) {
	for _, runID := range []string{"", "  ", "###"} {
		_, _, err := GetRunCommentsFunc(t.Context(), nil, GetRunCommentsArguments{RunID: runID})
		require.EqualError(t, err, "run_id must not be blank")
	}
}

func TestGetRunCommentsToolOutputSchema(t *testing.T) {
	schema, ok := GetRunCommentsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Equal(t, "object", items.Items.Type)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	responses := []RunCommentsResponse{
		{Items: []RunCommentSummary{}},
		{Items: []RunCommentSummary{{ID: "comment-1", Body: "approved"}}},
	}
	for _, response := range responses {
		data, err := json.Marshal(response)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}
