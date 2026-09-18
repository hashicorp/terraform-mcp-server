// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProjectTool(t *testing.T) {
	tool := GetProjectTool()

	assert.Equal(t, "get_project", tool.Name)
	assert.Contains(t, tool.Description, "Fetches detailed information about a Terraform project")
	assert.Nil(t, tool.InputSchema, "get_project relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get a Terraform project by ID", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

// get_project leaves InputSchema nil so the SDK derives it from
// GetProjectArguments. Derive it the same way to confirm the struct tags still
// make project_id a required string.
func TestGetProjectArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetProjectArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"project_id"}, schema.Required)

	projectID := schema.Properties["project_id"]
	require.NotNil(t, projectID)
	assert.Equal(t, "string", projectID.Type)
	assert.Contains(t, projectID.Description, "The ID of the project to fetch")
}

func TestGetProjectFunc_RequiresProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
	}{
		{name: "empty", projectID: ""},
		{name: "whitespace only", projectID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetProjectFunc(t.Context(), nil, GetProjectArguments{ProjectID: tt.projectID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "project_id must not be blank")
		})
	}
}
