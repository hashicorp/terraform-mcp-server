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

func TestGetWorkspaceDetailsTool(t *testing.T) {
	tool := GetWorkspaceDetailsTool()
	assert.Equal(t, "get_workspace_details", tool.Name)
	assert.Contains(t, tool.Description, "detailed information about a specific Terraform workspace")
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetWorkspaceDetailsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetWorkspaceDetailsArguments](nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)
}

func TestGetWorkspaceDetailsToolOutputSchema(t *testing.T) {
	schema, ok := GetWorkspaceDetailsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	variables := schema.Properties["variables"]
	require.NotNil(t, variables)
	assert.Equal(t, "array", variables.Type)
	assert.Contains(t, schema.Required, "variables")
	assert.Contains(t, schema.Required, "readme")

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	response := GetWorkspaceDetailsResponse{
		WorkspaceDetails: WorkspaceDetails{TriggerPrefixes: []string{}, TagNames: []string{}},
		Variables:        []VariableSummary{},
		Readme:           "README",
	}
	data, err := json.Marshal(response)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal(data, &value))
	assert.NoError(t, resolved.Validate(&value))
}

func TestGetWorkspaceDetailsFuncRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		input   GetWorkspaceDetailsArguments
		wantErr string
	}{
		{input: GetWorkspaceDetailsArguments{WorkspaceName: "workspace"}, wantErr: "terraform_org_name must not be blank"},
		{input: GetWorkspaceDetailsArguments{TerraformOrgName: "organization", WorkspaceName: "  "}, wantErr: "workspace_name must not be blank"},
	}

	for _, test := range tests {
		_, _, err := GetWorkspaceDetailsFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}

func TestDefaultWorkspaceReadmeFor(t *testing.T) {
	readme := defaultWorkspaceReadmeFor("organization", "workspace")
	assert.Contains(t, readme, `organization = "organization"`)
	assert.Contains(t, readme, `name = "workspace"`)
	assert.NotContains(t, readme, "<<your-terraform-org>>")
	assert.NotContains(t, readme, "<<your-terraform-workspace>>")
}
