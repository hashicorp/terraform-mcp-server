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

func TestListWorkspacePolicySetsTool(t *testing.T) {
	tool := ListWorkspacePolicySetsTool()

	assert.Equal(t, "list_workspace_policy_sets", tool.Name)
	assert.Equal(t, "List Terraform workspaces policy sets", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	assert.Nil(t, tool.InputSchema, "list_workspace_policy_sets relies on the SDK deriving its input schema")
}

func TestListWorkspacePolicySetsInputSchema(t *testing.T) {
	schema, err := jsonschema.For[PolicySetsArguments](nil)
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

func TestListWorkspacePolicySetsOutputSchema(t *testing.T) {
	schema, ok := ListWorkspacePolicySetsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"items"}, schema.PropertyOrder)
	assert.Equal(t, []string{"items"}, schema.Required)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Empty(t, items.Types)
	require.NotNil(t, items.Items)
	assert.Equal(t, "object", items.Items.Type)
	assert.Equal(t, []string{"id", "name", "description", "kind", "global", "reason"}, items.Items.PropertyOrder)
	assert.Equal(t, []string{"id", "name", "description", "kind", "global", "reason"}, items.Items.Required)
	require.NotNil(t, items.Items.AdditionalProperties)
	assert.NotNil(t, items.Items.AdditionalProperties.Not)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []PolicySetsSummaryList{
		{Items: make([]PolicySetsSummary, 0)},
		{Items: []PolicySetsSummary{{
			ID:          "polset-123",
			Name:        "security",
			Description: "Security policies",
			Kind:        "sentinel",
			Global:      true,
			Reason:      "global",
		}}},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestListWorkspacePolicySetsRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   PolicySetsArguments
		wantErr string
	}{
		{
			name:    "empty organization name",
			input:   PolicySetsArguments{WorkspaceID: "ws-2HRvNs49EWPjDqT1"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "whitespace-only organization name",
			input:   PolicySetsArguments{TerraformOrgName: "   ", WorkspaceID: "ws-2HRvNs49EWPjDqT1"},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name:    "empty workspace ID",
			input:   PolicySetsArguments{TerraformOrgName: "test-org"},
			wantErr: "workspace_id must not be blank",
		},
		{
			name:    "whitespace-only workspace ID",
			input:   PolicySetsArguments{TerraformOrgName: "test-org", WorkspaceID: "\t\n "},
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
