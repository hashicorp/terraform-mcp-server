// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkspaceTool(t *testing.T) {
	tool := UpdateWorkspaceTool()
	assert.Equal(t, "update_workspace", tool.Name)
	assert.Contains(t, tool.Description, "Updates an existing Terraform workspace")
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestUpdateWorkspaceArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[UpdateWorkspaceArguments](nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)
	assert.Contains(t, schema.Properties["tags"].Description, "ignored")
}

func TestUpdateWorkspaceFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		input   UpdateWorkspaceArguments
		wantErr string
	}{
		{input: UpdateWorkspaceArguments{WorkspaceName: "workspace"}, wantErr: "terraform_org_name must not be blank"},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "  "}, wantErr: "workspace_name must not be blank"},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", ExecutionMode: "invalid"}, wantErr: `execution_mode "invalid" must be one of: remote, local, agent`},
	}

	for _, test := range tests {
		_, _, err := UpdateWorkspaceFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}

func TestWorkspaceUpdateOptions(t *testing.T) {
	options, err := workspaceUpdateOptions(UpdateWorkspaceArguments{
		AutoApply:           "TRUE",
		QueueAllRuns:        "not-true",
		SpeculativeEnabled:  "false",
		FileTriggersEnabled: "TRUE",
		ExecutionMode:       "AGENT",
		TriggerPrefixes:     " modules/, environments/ ",
		Tags:                "ignored",
	})
	require.NoError(t, err)

	assert.True(t, *options.AutoApply)
	assert.False(t, *options.QueueAllRuns)
	assert.False(t, *options.SpeculativeEnabled)
	assert.True(t, *options.FileTriggersEnabled)
	assert.Equal(t, "agent", *options.ExecutionMode)
	assert.Equal(t, []string{"modules/", "environments/"}, options.TriggerPrefixes)
}
