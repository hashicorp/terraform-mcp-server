// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteProjectTool(t *testing.T) {
	tool := DeleteProjectTool()

	assert.Equal(t, "delete_project", tool.Name)
	assert.Contains(t, tool.Description, "Deletes a Terraform project by ID")
	assert.Nil(t, tool.InputSchema, "delete_project relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Delete a Terraform project by ID", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

// delete_project leaves InputSchema nil so the SDK derives it from
// DeleteProjectArguments. Derive it the same way to confirm the struct tags
// still make project_id a required string.
func TestDeleteProjectArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[DeleteProjectArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"project_id"}, schema.Required)

	projectID := schema.Properties["project_id"]
	require.NotNil(t, projectID)
	assert.Equal(t, "string", projectID.Type)
	assert.Contains(t, projectID.Description, "The ID of the Project to delete")
}

func TestDeleteProjectFunc_RequiresProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
	}{
		{name: "empty", projectID: ""},
		{name: "whitespace only", projectID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DeleteProjectFunc(t.Context(), nil, DeleteProjectArguments{ProjectID: tt.projectID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "project_id must not be blank")
		})
	}
}
