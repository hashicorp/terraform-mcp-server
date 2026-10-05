// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTeamTool(t *testing.T) {
	tool := CreateTeamTool()

	assert.Equal(t, "create_team", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new team")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Create a new team in a Terraform organization", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateTeamToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateTeamTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "team_name", "visibility"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "team_name"}, schema.Required)

	visibility := schema.Properties["visibility"]
	require.NotNil(t, visibility)
	assert.Equal(t, []any{"secret", "organization"}, visibility.Enum)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

// validates before reaching the TFE client, so it runs without one.
func TestCreateTeamFunc_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateTeamArguments
		wantErr string
	}{
		{name: "blank org", input: CreateTeamArguments{TerraformOrgName: " ", TeamName: "x"}, wantErr: "terraform_org_name must not be blank"},
		{name: "blank team name", input: CreateTeamArguments{TerraformOrgName: "org", TeamName: " "}, wantErr: "team_name must not be blank"},
		{name: "bad visibility", input: CreateTeamArguments{TerraformOrgName: "org", TeamName: "x", Visibility: "public"}, wantErr: "invalid visibility"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CreateTeamFunc(t.Context(), nil, tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
