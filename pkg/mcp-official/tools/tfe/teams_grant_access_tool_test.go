// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrantTeamAccessTool(t *testing.T) {
	tool := GrantTeamAccessTool()

	assert.Equal(t, "grant_team_access", tool.Name)
	assert.Contains(t, tool.Description, "Grants a team permission")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Grant team access to a workspace or project", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGrantTeamAccessToolInputSchema(t *testing.T) {
	schema := inputSchema(t, GrantTeamAccessTool().InputSchema)

	assert.Equal(t, []string{"team_id", "access_level", "workspace_id", "project_id"}, schema.PropertyOrder)
	assert.Equal(t, []string{"team_id", "access_level"}, schema.Required)

	accessLevel := schema.Properties["access_level"]
	require.NotNil(t, accessLevel)
	assert.Equal(t, []any{"admin", "read", "write", "plan", "maintain"}, accessLevel.Enum)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

// validates before reaching the TFE client, so it runs without one.
func TestGrantTeamAccessFunc_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   GrantTeamAccessArguments
		wantErr string
	}{
		{name: "blank team id", input: GrantTeamAccessArguments{TeamID: " ", AccessLevel: "read", WorkspaceID: "ws-1"}, wantErr: "team_id must not be blank"},
		{name: "blank access level", input: GrantTeamAccessArguments{TeamID: "team-1", AccessLevel: " ", WorkspaceID: "ws-1"}, wantErr: "access_level must not be blank"},
		{name: "neither workspace nor project", input: GrantTeamAccessArguments{TeamID: "team-1", AccessLevel: "read"}, wantErr: "must be provided"},
		{name: "both workspace and project", input: GrantTeamAccessArguments{TeamID: "team-1", AccessLevel: "read", WorkspaceID: "ws-1", ProjectID: "prj-1"}, wantErr: "not both"},
		{name: "plan on a project", input: GrantTeamAccessArguments{TeamID: "team-1", AccessLevel: "plan", ProjectID: "prj-1"}, wantErr: "invalid project access level"},
		{name: "maintain on a workspace", input: GrantTeamAccessArguments{TeamID: "team-1", AccessLevel: "maintain", WorkspaceID: "ws-1"}, wantErr: "invalid workspace access level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GrantTeamAccessFunc(t.Context(), nil, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
