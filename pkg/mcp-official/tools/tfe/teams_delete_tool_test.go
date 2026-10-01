// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteTeamTool(t *testing.T) {
	tool := DeleteTeamTool()

	assert.Equal(t, "delete_team", tool.Name)
	assert.Contains(t, tool.Description, "Permanently deletes a Terraform team")
	assert.Nil(t, tool.InputSchema, "delete_team relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, `Delete a Terraform team by "team_id"`, tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestDeleteTeamArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[DeleteTeamArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"team_id"}, schema.Required)

	teamID := schema.Properties["team_id"]
	require.NotNil(t, teamID)
	assert.Equal(t, "string", teamID.Type)
	assert.Contains(t, teamID.Description, "The ID of the team to delete")
}

func TestDeleteTeamFunc_RequiresTeamID(t *testing.T) {
	tests := []struct {
		name   string
		teamID string
	}{
		{name: "empty", teamID: ""},
		{name: "whitespace only", teamID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DeleteTeamFunc(t.Context(), nil, DeleteTeamArguments{TeamID: tt.teamID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "team_id must not be blank")
		})
	}
}
