// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNoCodeWorkspaceTool(t *testing.T) {
	tool := CreateNoCodeWorkspaceTool()
	assert.Equal(t, "create_no_code_workspace", tool.Name)
	assert.Contains(t, tool.Description, "Creates a new Terraform No Code module workspace")

	schema := inputSchema(t, tool.InputSchema)
	assert.Equal(t, []string{"no_code_module_id", "workspace_name", "project_id", "auto_apply"}, schema.PropertyOrder)
	assert.Equal(t, []string{"no_code_module_id", "workspace_name", "project_id"}, schema.Required)
	assert.Equal(t, "string", schema.Properties["no_code_module_id"].Type)
	assert.Equal(t, `^nocode-`, schema.Properties["no_code_module_id"].Pattern)
	assert.Equal(t, "boolean", schema.Properties["auto_apply"].Type)
	assert.JSONEq(t, "false", string(schema.Properties["auto_apply"].Default))
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	require.NotNil(t, tool.Annotations)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	assert.False(t, tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Annotations.DestructiveHint)
}

func TestCreateNoCodeWorkspaceOutputSchema(t *testing.T) {
	schema := inputSchema(t, CreateNoCodeWorkspaceTool().OutputSchema)
	assert.Equal(t, []string{"workspace_id", "workspace_name", "project_id", "no_code_module_id", "auto_apply"}, schema.PropertyOrder)
	assert.Equal(t, schema.PropertyOrder, schema.Required)
}

func TestCreateNoCodeWorkspaceFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateNoCodeWorkspaceArguments
		wantErr string
	}{
		{name: "blank module", input: CreateNoCodeWorkspaceArguments{WorkspaceName: "workspace", ProjectID: "project"}, wantErr: "no_code_module_id must not be blank"},
		{name: "invalid module prefix", input: CreateNoCodeWorkspaceArguments{NoCodeModuleID: "mod-123", WorkspaceName: "workspace", ProjectID: "project"}, wantErr: "no_code_module_id must start with 'nocode-'"},
		{name: "blank workspace", input: CreateNoCodeWorkspaceArguments{NoCodeModuleID: "nocode-123", ProjectID: "project"}, wantErr: "workspace_name must not be blank"},
		{name: "blank project", input: CreateNoCodeWorkspaceArguments{NoCodeModuleID: "nocode-123", WorkspaceName: "workspace"}, wantErr: "project_id must not be blank"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := CreateNoCodeWorkspaceFunc(t.Context(), nil, test.input)
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestBuildNoCodeElicitationSchema(t *testing.T) {
	metadata := testNoCodeModuleMetadata(t)
	schema := buildNoCodeElicitationSchema(metadata, &tfe.RegistryNoCodeModule{})

	assert.Equal(t, []string{"name"}, schema.schema.Required)
	assert.Equal(t, []string{"name", "description"}, schema.schema.PropertyOrder)
	assert.Equal(t, "string", schema.schema.Properties["name"].Type)
	assert.Equal(t, "string", schema.schema.Properties["description"].Type)
}

func TestExtractNoCodeVariables(t *testing.T) {
	metadata := testNoCodeModuleMetadata(t)
	schema := buildNoCodeElicitationSchema(metadata, &tfe.RegistryNoCodeModule{})

	variables, err := extractNoCodeVariables(map[string]any{"name": "example"}, schema)
	require.NoError(t, err)
	require.Len(t, variables, 1)
	assert.Equal(t, "name", variables[0].Key)
	assert.Equal(t, "example", variables[0].Value)

	variables, err = extractNoCodeVariables(map[string]any{"name": "example", "description": ""}, schema)
	require.NoError(t, err)
	assert.Len(t, variables, 1)
}

func TestProcessNoCodeElicitationResponse(t *testing.T) {
	metadata := testNoCodeModuleMetadata(t)
	schema := buildNoCodeElicitationSchema(metadata, &tfe.RegistryNoCodeModule{})

	for _, test := range []struct {
		action  string
		wantErr string
	}{
		{action: "decline", wantErr: "workspace creation declined by user"},
		{action: "cancel", wantErr: "workspace creation cancelled by user"},
		{action: "unknown", wantErr: "unexpected elicitation response action: unknown"},
	} {
		t.Run(test.action, func(t *testing.T) {
			_, err := processNoCodeElicitationResponse(&mcp.ElicitResult{Action: test.action}, schema)
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestNoCodeVariableConversions(t *testing.T) {
	tests := []struct {
		name         string
		variableType string
		value        any
		want         string
		wantError    string
	}{
		{name: "string", variableType: "string", value: "value", want: "value"},
		{name: "number", variableType: "number", value: float64(1.5), want: "1.5"},
		{name: "boolean", variableType: "boolean", value: true, want: "true"},
		{name: "invalid string", variableType: "string", value: true, wantError: "must be a string"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := convertNoCodeVariableValue("value", test.variableType, test.value)
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func testNoCodeModuleMetadata(t *testing.T) *tfeclient.ModuleMetadata {
	t.Helper()
	metadataJSON := []byte(`{
		"data": {"attributes": {"input-variables": [
			{"name": "name", "type": "string", "required": true},
			{"name": "description", "type": "string", "required": false}
		]}}
	}`)
	var metadata tfeclient.ModuleMetadata
	require.NoError(t, json.Unmarshal(metadataJSON, &metadata))
	return &metadata
}
