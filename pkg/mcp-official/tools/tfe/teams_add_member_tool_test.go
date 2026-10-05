// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTeamMemberTool(t *testing.T) {
	tool := AddTeamMemberTool()

	assert.Equal(t, "add_team_member", tool.Name)
	assert.Contains(t, tool.Description, "Adds a single member")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Add member to a Terraform team", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestAddTeamMemberToolInputSchema(t *testing.T) {
	schema := inputSchema(t, AddTeamMemberTool().InputSchema)

	assert.Equal(t, []string{"team_id", "username", "organization_membership_id"}, schema.PropertyOrder)
	assert.Equal(t, []string{"team_id"}, schema.Required)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

// validates before reaching the TFE client, so it runs without one.
func TestAddTeamMemberFunc_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   AddTeamMemberArguments
		wantErr string
	}{
		{name: "blank team id", input: AddTeamMemberArguments{TeamID: " ", Username: "u"}, wantErr: "team_id must not be blank"},
		{name: "neither username nor membership id", input: AddTeamMemberArguments{TeamID: "team-1"}, wantErr: "must be provided"},
		{name: "both username and membership id", input: AddTeamMemberArguments{TeamID: "team-1", Username: "u", OrganizationMembershipID: "ou-1"}, wantErr: "not both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := AddTeamMemberFunc(t.Context(), nil, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
