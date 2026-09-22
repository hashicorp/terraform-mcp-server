// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWorkspaceTagsTool(t *testing.T) {
	tool := CreateWorkspaceTagsTool()

	assert.Equal(t, "create_workspace_tags", tool.Name)
	assert.Equal(t, "Create Terraform workspace tags", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint, "the tool writes tag bindings")
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint, "adding tags is additive")

	// The arguments carry no constraints beyond being required, so the SDK infers
	// the input schema from the struct tags.
	assert.Nil(t, tool.InputSchema)
}

func TestCreateWorkspaceTagsInputSchema(t *testing.T) {
	schema, err := jsonschema.For[CreateWorkspaceTagsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.ElementsMatch(t, []string{"terraform_org_name", "workspace_name", "tags"}, schema.Required)

	for _, name := range []string{"terraform_org_name", "workspace_name", "tags"} {
		property, ok := schema.Properties[name]
		require.True(t, ok, "schema should declare property %q", name)
		assert.Equal(t, "string", property.Type)
		assert.NotEmpty(t, property.Description, "property %q should describe itself to the model", name)
	}

	assert.Contains(t, schema.Properties["tags"].Description, "key:value",
		"the tags description must teach the model the key:value form the parser accepts")
}

func TestCreateWorkspaceTagsRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateWorkspaceTagsArguments
		wantErr string
	}{
		{
			name:    "empty organization name",
			input:   CreateWorkspaceTagsArguments{WorkspaceName: "ws", Tags: "a"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "whitespace-only organization name",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "   ", WorkspaceName: "ws", Tags: "a"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "empty workspace name",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", Tags: "a"},
			wantErr: "workspace_name must not be blank",
		},
		{
			name:    "whitespace-only workspace name",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "\t ", Tags: "a"},
			wantErr: "workspace_name must not be blank",
		},
		{
			name:    "empty tags",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "ws"},
			wantErr: "tags must not be blank",
		},
		{
			name:    "whitespace-only tags",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "ws", Tags: "  "},
			wantErr: "tags must not be blank",
		},
		{
			// Non-blank but parses to nothing, so the call would otherwise report
			// success for a request that changed no tags.
			name:    "tags of separators only",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "ws", Tags: ", ,"},
			wantErr: `tags ", ," contained no valid tag names`,
		},
		{
			name:    "tags with blank keys only",
			input:   CreateWorkspaceTagsArguments{TerraformOrgName: "org", WorkspaceName: "ws", Tags: ":value"},
			wantErr: `tags ":value" contained no valid tag names`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A nil request is safe because validation runs before client acquisition.
			result, out, err := CreateWorkspaceTagsFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
			assert.Nil(t, result)
			assert.Nil(t, out)
		})
	}
}
