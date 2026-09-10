// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProjectTool(t *testing.T) {
	tool := CreateProjectTool()

	assert.Equal(t, "create_project", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new Terraform project")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Create a new Terraform project", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateProjectToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateProjectTool().InputSchema)

	assert.Equal(t, []string{"terraform_org_name", "project_name", "description", "default_execution_mode"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "project_name"}, schema.Required)

	projectName := schema.Properties["project_name"]
	require.NotNil(t, projectName)
	require.NotNil(t, projectName.MinLength)
	assert.Equal(t, 3, *projectName.MinLength)
	require.NotNil(t, projectName.MaxLength)
	assert.Equal(t, 40, *projectName.MaxLength)
	assert.Equal(t, `^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$`, projectName.Pattern)

	description := schema.Properties["description"]
	require.NotNil(t, description)
	require.NotNil(t, description.MaxLength)
	assert.Equal(t, 256, *description.MaxLength)

	executionMode := schema.Properties["default_execution_mode"]
	require.NotNil(t, executionMode)
	assert.Equal(t, []any{executionModeLocal, executionModeAgent, executionModeRemote}, executionMode.Enum)

	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateProjectFunc_ValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		input CreateProjectArguments
		want  string
	}{
		{
			name:  "blank organization",
			input: CreateProjectArguments{TerraformOrgName: "   ", ProjectName: "project"},
			want:  "terraform_org_name must not be blank",
		},
		{
			name:  "blank project name",
			input: CreateProjectArguments{TerraformOrgName: "org", ProjectName: "   "},
			want:  "project_name must not be blank",
		},
		{
			name:  "unsupported execution mode",
			input: CreateProjectArguments{TerraformOrgName: "org", ProjectName: "project", DefaultExecutionMode: "cloud"},
			want:  `invalid default_execution_mode "cloud": must be one of local, agent, remote`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CreateProjectFunc(t.Context(), nil, tt.input)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
