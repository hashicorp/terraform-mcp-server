// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWorkspaceTool(t *testing.T) {
	tool := CreateWorkspaceTool()
	assert.Equal(t, "create_workspace", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new Terraform workspace")
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Create a new Terraform workspace", tool.Annotations.Title)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestCreateWorkspaceToolInputSchema(t *testing.T) {
	schema := inputSchema(t, CreateWorkspaceTool().InputSchema)
	assert.Equal(t, []string{
		"terraform_org_name", "workspace_name", "description", "terraform_version",
		"working_directory", "auto_apply", "execution_mode", "project_id",
		"vcs_repo_identifier", "vcs_repo_branch", "vcs_repo_oauth_token_id", "tags",
	}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)
	assert.Equal(t, "boolean", schema.Properties["auto_apply"].Type)
	assert.JSONEq(t, "false", string(schema.Properties["auto_apply"].Default))
	assert.Equal(t, enumOf(validExecutionModes...), schema.Properties["execution_mode"].Enum)
	assert.Equal(t, "string", schema.Properties["vcs_repo_oauth_token_id"].Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateWorkspaceFuncRejectsBlankArguments(t *testing.T) {
	tests := []struct {
		input   CreateWorkspaceArguments
		wantErr string
	}{
		{input: CreateWorkspaceArguments{WorkspaceName: "workspace"}, wantErr: "terraform_org_name must not be blank"},
		{input: CreateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "  "}, wantErr: "workspace_name must not be blank"},
		{input: CreateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", ExecutionMode: "invalid"}, wantErr: `execution_mode "invalid" must be one of: local, remote`},
		{input: CreateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", ExecutionMode: "agent"}, wantErr: `execution_mode "agent" must be one of: local, remote`},
		{input: CreateWorkspaceArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", VCSRepoIdentifier: "org/repo"}, wantErr: "vcs_repo_oauth_token_id is required when vcs_repo_identifier is provided"},
	}

	for _, test := range tests {
		_, _, err := CreateWorkspaceFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}

func TestWorkspaceCreateOptions(t *testing.T) {
	options, err := workspaceCreateOptions(CreateWorkspaceArguments{
		AutoApply:           true,
		ExecutionMode:       executionModeLocal,
		Tags:                " production, ,team-platform ",
		VCSRepoIdentifier:   "org/repo",
		VCSRepoBranch:       "feature",
		VCSRepoOAuthTokenID: "ot-123",
	}, "workspace")
	require.NoError(t, err)

	require.NotNil(t, options.Name)
	assert.Equal(t, "workspace", *options.Name)
	require.NotNil(t, options.AutoApply)
	assert.True(t, *options.AutoApply)
	require.NotNil(t, options.ExecutionMode)
	assert.Equal(t, "local", *options.ExecutionMode)
	require.Len(t, options.Tags, 2)
	assert.Equal(t, "production", options.Tags[0].Name)
	assert.Equal(t, "team-platform", options.Tags[1].Name)
	require.NotNil(t, options.VCSRepo)
	assert.Equal(t, "org/repo", *options.VCSRepo.Identifier)
	assert.Equal(t, workspaceSourceName, *options.SourceName)
}
