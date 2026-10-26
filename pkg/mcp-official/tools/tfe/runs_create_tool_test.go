// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRunTools(t *testing.T) {
	safeTool := CreateRunSafeTool()
	assert.Equal(t, "create_run", safeTool.Name)
	assert.Contains(t, safeTool.Description, "Creates a new Terraform run")
	require.NotNil(t, safeTool.Annotations)
	require.NotNil(t, safeTool.Annotations.DestructiveHint)
	assert.False(t, *safeTool.Annotations.DestructiveHint)

	tool := CreateRunTool()
	assert.Equal(t, "create_run", tool.Name)
	require.NotNil(t, tool.Annotations)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestCreateRunToolInputSchemas(t *testing.T) {
	tests := []struct {
		name         string
		tool         *mcp.Tool
		wantRunTypes []any
	}{
		{name: "safe", tool: CreateRunSafeTool(), wantRunTypes: enumOf(safeRunTypes...)},
		{name: "all", tool: CreateRunTool(), wantRunTypes: enumOf(allRunTypes...)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := inputSchema(t, test.tool.InputSchema)
			assert.Equal(t, []string{"terraform_org_name", "workspace_name", "run_type", "message"}, schema.PropertyOrder)
			assert.Equal(t, []string{"terraform_org_name", "workspace_name"}, schema.Required)
			assert.Equal(t, test.wantRunTypes, schema.Properties["run_type"].Enum)
			assert.JSONEq(t, `"plan_and_apply"`, string(schema.Properties["run_type"].Default))
			assert.JSONEq(t, `"Triggered via Terraform MCP Server"`, string(schema.Properties["message"].Default))
			require.NotNil(t, schema.AdditionalProperties)
			assert.NotNil(t, schema.AdditionalProperties.Not)
		})
	}
}

func TestCreateRunToolOutputSchema(t *testing.T) {
	schema := inputSchema(t, CreateRunTool().OutputSchema)
	assert.Equal(t, []string{"run_id", "workspace_id", "status", "message"}, schema.PropertyOrder)
	assert.Equal(t, schema.PropertyOrder, schema.Required)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestCreateRunFuncsRejectInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		call    func() error
		wantErr string
	}{
		{
			name: "blank organization",
			call: func() error {
				_, _, err := CreateRunSafeFunc(t.Context(), nil, CreateRunArguments{WorkspaceName: "workspace"})
				return err
			},
			wantErr: "terraform_org_name must not be blank",
		},
		{
			name: "blank workspace",
			call: func() error {
				_, _, err := CreateRunSafeFunc(t.Context(), nil, CreateRunArguments{TerraformOrgName: "organization", WorkspaceName: "  "})
				return err
			},
			wantErr: "workspace_name must not be blank",
		},
		{
			name: "unknown run type",
			call: func() error {
				_, _, err := CreateRunFunc(t.Context(), nil, CreateRunArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", RunType: "unknown"})
				return err
			},
			wantErr: `run_type "unknown" must be one of: plan_and_apply, refresh_state, plan_only, allow_empty_apply, auto_approve, is_destroy`,
		},
		{
			name: "safe auto approve",
			call: func() error {
				_, _, err := CreateRunSafeFunc(t.Context(), nil, CreateRunArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", RunType: runTypeAutoApprove})
				return err
			},
			wantErr: `run_type "auto_approve" must be one of: plan_and_apply, refresh_state, plan_only, allow_empty_apply`,
		},
		{
			name: "safe destroy",
			call: func() error {
				_, _, err := CreateRunSafeFunc(t.Context(), nil, CreateRunArguments{TerraformOrgName: "organization", WorkspaceName: "workspace", RunType: runTypeDestroy})
				return err
			},
			wantErr: `run_type "is_destroy" must be one of: plan_and_apply, refresh_state, plan_only, allow_empty_apply`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.EqualError(t, test.call(), test.wantErr)
		})
	}
}

func TestRunCreateOptions(t *testing.T) {
	workspace := &tfe.Workspace{ID: "ws-1"}
	tests := []struct {
		runType string
		assert  func(*testing.T, tfe.RunCreateOptions)
	}{
		{runType: runTypePlanAndApply, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.AutoApply)
			assert.False(t, *options.AutoApply)
		}},
		{runType: runTypeRefreshState, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.RefreshOnly)
			assert.True(t, *options.RefreshOnly)
		}},
		{runType: runTypePlanOnly, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.PlanOnly)
			assert.True(t, *options.PlanOnly)
		}},
		{runType: runTypeAllowEmptyApply, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.AllowEmptyApply)
			assert.True(t, *options.AllowEmptyApply)
		}},
		{runType: runTypeAutoApprove, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.AutoApply)
			assert.True(t, *options.AutoApply)
		}},
		{runType: runTypeDestroy, assert: func(t *testing.T, options tfe.RunCreateOptions) {
			require.NotNil(t, options.IsDestroy)
			assert.True(t, *options.IsDestroy)
		}},
	}

	for _, test := range tests {
		t.Run(test.runType, func(t *testing.T) {
			options := runCreateOptions(workspace, test.runType, nil)
			assert.Same(t, workspace, options.Workspace)
			require.NotNil(t, options.Message)
			assert.Equal(t, defaultRunMessage, *options.Message)
			test.assert(t, options)
		})
	}
}

func TestRunCreateOptionsMessage(t *testing.T) {
	message := "custom message"
	options := runCreateOptions(&tfe.Workspace{ID: "ws-1"}, runTypePlanOnly, &message)
	require.NotNil(t, options.Message)
	assert.Equal(t, message, *options.Message)

	empty := ""
	options = runCreateOptions(&tfe.Workspace{ID: "ws-1"}, runTypePlanOnly, &empty)
	assert.Nil(t, options.Message)
}
