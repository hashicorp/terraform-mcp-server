// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorkspacePolicySetsTool(t *testing.T) {
	tool := ListWorkspacePolicySetsTool()

	assert.Equal(t, "list_workspace_policy_sets", tool.Name)
	assert.Equal(t, "List Terraform workspaces policy sets", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	// The arguments carry no constraints beyond being required, so the SDK infers
	// the input schema from the struct tags.
	assert.Nil(t, tool.InputSchema)
}

func TestListWorkspacePolicySetsInputSchema(t *testing.T) {
	schema, err := jsonschema.For[ListWorkspacePolicySetsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.ElementsMatch(t, []string{"terraform_org_name", "workspace_id"}, schema.Required)

	for _, name := range []string{"terraform_org_name", "workspace_id"} {
		property, ok := schema.Properties[name]
		require.True(t, ok, "schema should declare property %q", name)
		assert.Equal(t, "string", property.Type)
		assert.NotEmpty(t, property.Description, "property %q should describe itself to the model", name)
	}
}

// TestListWorkspacePolicySetsSchemasAreObjectRooted checks the shape strict clients
// validate. A non-object root in either schema is dropped silently at discovery, so the
// tool would simply not exist for those clients while every other test still passed.
func TestListWorkspacePolicySetsSchemasAreObjectRooted(t *testing.T) {
	listed := listedTool(t, ListWorkspacePolicySetsTool(), ListWorkspacePolicySetsFunc)

	objectSchema(t, listed.InputSchema, "input schema")

	outputProperties := objectSchema(t, listed.OutputSchema, "output schema")
	assert.Contains(t, outputProperties, "items", "policy sets belong under items, not at the root")
}

func TestListWorkspacePolicySetsRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   ListWorkspacePolicySetsArguments
		wantErr string
	}{
		{
			name:    "empty organization name",
			input:   ListWorkspacePolicySetsArguments{WorkspaceID: "ws-2HRvNs49EWPjDqT1"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "whitespace-only organization name",
			input:   ListWorkspacePolicySetsArguments{TerraformOrgName: "   ", WorkspaceID: "ws-2HRvNs49EWPjDqT1"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "empty workspace ID",
			input:   ListWorkspacePolicySetsArguments{TerraformOrgName: "test-org"},
			wantErr: "workspace_id must not be blank",
		},
		{
			name:    "whitespace-only workspace ID",
			input:   ListWorkspacePolicySetsArguments{TerraformOrgName: "test-org", WorkspaceID: "\t\n "},
			wantErr: "workspace_id must not be blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A nil request is safe because validation runs before client acquisition.
			result, out, err := ListWorkspacePolicySetsFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
			assert.Nil(t, result)
			assert.Nil(t, out)
		})
	}
}
