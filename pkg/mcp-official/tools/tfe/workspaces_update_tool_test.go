// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkspaceTool(t *testing.T) {
	tool := UpdateWorkspaceTool()
	assert.Equal(t, "update_workspace", tool.Name)
	assert.Contains(t, tool.Description, "Updates an existing Terraform workspace")
	assert.Contains(t, tool.Description, "create_workspace_tags")
	require.NotNil(t, tool.Annotations)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestUpdateWorkspaceToolInputSchema(t *testing.T) {
	schema := inputSchema(t, UpdateWorkspaceTool().InputSchema)
	assert.Equal(t, []string{
		"terraform_org_name", "workspace_name", "new_name", "description",
		"terraform_version", "working_directory", "auto_apply", "execution_mode",
		"queue_all_runs", "speculative_enabled", "trigger_prefixes", "file_triggers_enabled",
	}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)
	assert.Equal(t, "boolean", schema.Properties["auto_apply"].Type)
	assert.Equal(t, "boolean", schema.Properties["queue_all_runs"].Type)
	assert.Equal(t, "boolean", schema.Properties["speculative_enabled"].Type)
	assert.Equal(t, "boolean", schema.Properties["file_triggers_enabled"].Type)
	assert.Equal(t, enumOf(validExecutionModes...), schema.Properties["execution_mode"].Enum)
	assert.Equal(t, "array", schema.Properties["trigger_prefixes"].Type)
	assert.Equal(t, "string", schema.Properties["trigger_prefixes"].Items.Type)
	assert.NotContains(t, schema.Properties, "tags")
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestUpdateWorkspaceFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		input   UpdateWorkspaceArguments
		wantErr string
	}{
		{input: UpdateWorkspaceArguments{WorkspaceName: "workspace"}, wantErr: "terraform_org_name must not be blank"},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "  "}, wantErr: "workspace_name must not be blank"},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace"}, wantErr: "at least one workspace setting must be provided"},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", ExecutionMode: "invalid"}, wantErr: `execution_mode "invalid" must be one of: local, remote`},
		{input: UpdateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", ExecutionMode: "agent"}, wantErr: `execution_mode "agent" must be one of: local, remote`},
	}

	for _, test := range tests {
		_, _, err := UpdateWorkspaceFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}

func TestWorkspaceUpdateOptions(t *testing.T) {
	autoApply := true
	queueAllRuns := false
	speculativeEnabled := false
	fileTriggersEnabled := true
	options, err := workspaceUpdateOptions(UpdateWorkspaceArguments{
		AutoApply:           &autoApply,
		QueueAllRuns:        &queueAllRuns,
		SpeculativeEnabled:  &speculativeEnabled,
		FileTriggersEnabled: &fileTriggersEnabled,
		ExecutionMode:       executionModeRemote,
		TriggerPrefixes:     []string{"modules/", "environments/"},
	})
	require.NoError(t, err)

	assert.True(t, *options.AutoApply)
	assert.False(t, *options.QueueAllRuns)
	assert.False(t, *options.SpeculativeEnabled)
	assert.True(t, *options.FileTriggersEnabled)
	assert.Equal(t, executionModeRemote, *options.ExecutionMode)
	assert.Equal(t, []string{"modules/", "environments/"}, options.TriggerPrefixes)
}

func TestWorkspaceUpdateOptionsClearsFields(t *testing.T) {
	empty := ""
	prefixes := []string{}
	options, err := workspaceUpdateOptions(UpdateWorkspaceArguments{
		Description:      &empty,
		WorkingDirectory: &empty,
		TriggerPrefixes:  prefixes,
	})
	require.NoError(t, err)

	require.NotNil(t, options.Description)
	assert.Empty(t, *options.Description)
	require.NotNil(t, options.WorkingDirectory)
	assert.Empty(t, *options.WorkingDirectory)
	assert.NotNil(t, options.TriggerPrefixes)
	assert.Empty(t, options.TriggerPrefixes)
}
