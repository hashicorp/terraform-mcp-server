// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputSchema asserts the tool carries an explicit object schema and returns it.
func inputSchema(t *testing.T, schema any) *jsonschema.Schema {
	t.Helper()

	s, ok := schema.(*jsonschema.Schema)
	require.True(t, ok, "input schema should be a *jsonschema.Schema, got %T", schema)
	require.Equal(t, "object", s.Type)
	return s
}

func TestCreateRunSafeTool(t *testing.T) {
	tool := CreateRunSafeTool()

	assert.Equal(t, "create_run", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new Terraform run")

	// Check that destructive hint is false
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	assert.False(t, tool.Annotations.ReadOnlyHint)

	schema := inputSchema(t, tool.InputSchema)

	// Check required parameters
	assert.Contains(t, schema.Required, "terraform_org_name")
	assert.Contains(t, schema.Required, "workspace_name")
	assert.NotContains(t, schema.Required, "run_type")
	assert.NotContains(t, schema.Required, "message")

	// The safe variant must not offer the destructive run types
	runType := schema.Properties["run_type"]
	require.NotNil(t, runType)
	assert.Equal(t, []any{"plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply"}, runType.Enum)
	assert.JSONEq(t, `"plan_and_apply"`, string(runType.Default))
}

func TestCreateRunTool(t *testing.T) {
	tool := CreateRunTool()

	assert.Equal(t, "create_run", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new Terraform run")

	// Check that destructive hint is true
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
	assert.False(t, tool.Annotations.ReadOnlyHint)

	schema := inputSchema(t, tool.InputSchema)

	// Check required parameters
	assert.Contains(t, schema.Required, "terraform_org_name")
	assert.Contains(t, schema.Required, "workspace_name")

	// The destructive variant additionally offers auto_approve and is_destroy
	runType := schema.Properties["run_type"]
	require.NotNil(t, runType)
	assert.Contains(t, runType.Enum, "auto_approve")
	assert.Contains(t, runType.Enum, "is_destroy")
	assert.JSONEq(t, `"plan_and_apply"`, string(runType.Default))
}

// The safe variant rejects the destructive run types before it needs a Terraform
// client, so this exercises the guard without talking to TFE.
func TestCreateRunSafeFunc_RejectsDestructiveRunTypes(t *testing.T) {
	for _, runType := range []string{"auto_approve", "is_destroy"} {
		t.Run(runType, func(t *testing.T) {
			_, _, err := CreateRunSafeFunc(t.Context(), nil, CreateRunArguments{
				TerraformOrgName: "my-org",
				WorkspaceName:    "my-workspace",
				RunType:          runType,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), runType)
			assert.Contains(t, err.Error(), "Terraform operations are disabled")
		})
	}
}

func TestCreateRunFunc_RejectsUnknownRunType(t *testing.T) {
	_, _, err := CreateRunFunc(t.Context(), nil, CreateRunArguments{
		TerraformOrgName: "my-org",
		WorkspaceName:    "my-workspace",
		RunType:          "not-a-run-type",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run_type: not-a-run-type")
}

func TestCreateRunFunc_RequiresOrgAndWorkspace(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateRunArguments
		wantErr string
	}{
		{
			name:    "blank org name",
			input:   CreateRunArguments{TerraformOrgName: "   ", WorkspaceName: "my-workspace"},
			wantErr: "missing required input: terraform_org_name",
		},
		{
			name:    "blank workspace name",
			input:   CreateRunArguments{TerraformOrgName: "my-org", WorkspaceName: "   "},
			wantErr: "missing required input: workspace_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := CreateRunFunc(t.Context(), nil, tt.input)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
