// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachPolicySetToWorkspacesTool(t *testing.T) {
	tool := AttachPolicySetToWorkspacesTool()

	assert.Equal(t, "attach_policy_set_to_workspaces", tool.Name)
	assert.Equal(t, "Attach policy set to workspaces", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint, "the tool attaches a policy set")
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint, "attaching a policy set is additive")

	assert.Nil(t, tool.InputSchema)
}

func TestAttachPolicySetToWorkspacesInputSchema(t *testing.T) {
	schema, err := jsonschema.For[AttachPolicySetToWorkspacesArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.ElementsMatch(t, []string{"policy_set_id", "workspace_ids"}, schema.Required)

	for _, name := range []string{"policy_set_id", "workspace_ids"} {
		property, ok := schema.Properties[name]
		require.True(t, ok, "schema should declare property %q", name)
		assert.Equal(t, "string", property.Type)
		assert.NotEmpty(t, property.Description, "property %q should describe itself to the model", name)
	}
}

// TestAttachPolicySetToWorkspacesPublishesNoOutputSchema pins the text-result contract.
// This tool confirms a mutation in prose, so it declares no structured output. Retyping
// its output value to a struct would publish a schema and emit structured content the
// handler never fills, adding a phantom payload beside the confirmation text.
func TestAttachPolicySetToWorkspacesPublishesNoOutputSchema(t *testing.T) {
	listed := listedTool(t, AttachPolicySetToWorkspacesTool(), AttachPolicySetToWorkspacesFunc)

	objectSchema(t, listed.InputSchema, "input schema")
	assert.Nil(t, listed.OutputSchema, "a prose confirmation declares no structured output")
}

func TestAttachPolicySetToWorkspacesRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   AttachPolicySetToWorkspacesArguments
		wantErr string
	}{
		{
			name:    "empty policy set ID",
			input:   AttachPolicySetToWorkspacesArguments{WorkspaceIDs: "ws-1"},
			wantErr: "policy_set_id must not be blank",
		},
		{
			name:    "whitespace-only policy set ID",
			input:   AttachPolicySetToWorkspacesArguments{PolicySetID: "  ", WorkspaceIDs: "ws-1"},
			wantErr: "policy_set_id must not be blank",
		},
		{
			name:    "empty workspace IDs",
			input:   AttachPolicySetToWorkspacesArguments{PolicySetID: "polset-1"},
			wantErr: "workspace_ids must not be blank",
		},
		{
			name:    "whitespace-only workspace IDs",
			input:   AttachPolicySetToWorkspacesArguments{PolicySetID: "polset-1", WorkspaceIDs: "\t "},
			wantErr: "workspace_ids must not be blank",
		},
		{
			name:    "workspace IDs of separators only",
			input:   AttachPolicySetToWorkspacesArguments{PolicySetID: "polset-1", WorkspaceIDs: ", ,"},
			wantErr: "no valid workspace IDs provided",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// A nil request is safe because validation runs before client acquisition.
			result, out, err := AttachPolicySetToWorkspacesFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
			assert.Nil(t, result)
			assert.Nil(t, out)
		})
	}
}
