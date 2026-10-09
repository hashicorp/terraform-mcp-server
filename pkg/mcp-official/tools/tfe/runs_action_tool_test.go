// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionRunTool(t *testing.T) {
	tool := ActionRunTool()
	assert.Equal(t, "action_run", tool.Name)
	assert.Nil(t, tool.OutputSchema)
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestActionRunToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ActionRunTool().InputSchema)
	assert.Equal(t, []string{"run_action", "run_id", "comment"}, schema.PropertyOrder)
	assert.Equal(t, []string{"run_action", "run_id"}, schema.Required)
	assert.Equal(t, []any{"apply", "discard", "cancel"}, schema.Properties["run_action"].Enum)
	assert.JSONEq(t, `"`+defaultRunActionComment+`"`, string(schema.Properties["comment"].Default))
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestActionRunFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		input   ActionRunArguments
		wantErr string
	}{
		{input: ActionRunArguments{RunID: "run-1"}, wantErr: "run_action must not be blank"},
		{input: ActionRunArguments{RunAction: "apply"}, wantErr: "run_id must not be blank"},
		{input: ActionRunArguments{RunAction: "invalid", RunID: "run-1"}, wantErr: `run_action "invalid" must be one of: apply, discard, cancel`},
	}

	for _, test := range tests {
		_, _, err := ActionRunFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}
