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

func TestReadWorkspaceTagsTool(t *testing.T) {
	tool := ReadWorkspaceTagsTool()

	assert.Equal(t, "read_workspace_tags", tool.Name)
	assert.Equal(t, "Read Terraform workspace tags", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	assert.Nil(t, tool.InputSchema)
}

func TestReadWorkspaceTagsInputSchema(t *testing.T) {
	schema, err := jsonschema.For[ReadWorkspaceTagsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.ElementsMatch(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)

	for _, name := range []string{"terraform_org_name", "workspace_name"} {
		property, ok := schema.Properties[name]
		require.True(t, ok, "schema should declare property %q", name)
		assert.Equal(t, "string", property.Type)
		assert.NotEmpty(t, property.Description, "property %q should describe itself to the model", name)
	}
}

func TestReadWorkspaceTagsOutputSchema(t *testing.T) {
	schema, ok := ReadWorkspaceTagsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"workspace_name", "tags", "tag_bindings"}, schema.PropertyOrder)
	assert.Equal(t, []string{"workspace_name", "tags", "tag_bindings"}, schema.Required)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	for _, name := range []string{"tags", "tag_bindings"} {
		items := schema.Properties[name]
		require.NotNil(t, items)
		assert.Equal(t, "array", items.Type)
		assert.Empty(t, items.Types)
		require.NotNil(t, items.Items)
		assert.Equal(t, "string", items.Items.Type)
	}

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []ReadWorkspaceTagsResult{
		{WorkspaceName: "workspace", Tags: make([]string, 0), TagBindings: make([]string, 0)},
		{WorkspaceName: "workspace", Tags: []string{"production"}, TagBindings: []string{"env:staging"}},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestReadWorkspaceTagsRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   ReadWorkspaceTagsArguments
		wantErr string
	}{
		{
			name:    "empty organization name",
			input:   ReadWorkspaceTagsArguments{WorkspaceName: "ws"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "whitespace-only organization name",
			input:   ReadWorkspaceTagsArguments{TerraformOrgName: "   ", WorkspaceName: "ws"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "empty workspace name",
			input:   ReadWorkspaceTagsArguments{TerraformOrgName: "org"},
			wantErr: "workspace_name must not be blank",
		},
		{
			name:    "whitespace-only workspace name",
			input:   ReadWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "\t\n "},
			wantErr: "workspace_name must not be blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A nil request is safe because validation runs before client acquisition.
			result, out, err := ReadWorkspaceTagsFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
			assert.Nil(t, result)
			assert.Nil(t, out)
		})
	}
}
