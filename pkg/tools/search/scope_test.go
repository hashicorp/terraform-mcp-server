// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSearchScope(t *testing.T) {
	tests := []struct {
		name                    string
		organization, workspace string
		env                     map[string]string
		want                    SearchScope
		ready                   bool
	}{
		{"explicit user arguments override environment defaults", " user-org ", " user-workspace ", map[string]string{"TF_CLOUD_ORGANIZATION": " env-org ", "TF_MCP_QUERY_WORKSPACE": " env-workspace ", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"user-org", "argument"}, SearchScopeValue{"user-workspace", "argument"}}, true},
		{"arguments are trimmed fallback", " user-org ", " user-workspace ", map[string]string{"TF_CLOUD_ORGANIZATION": " ", "TF_MCP_QUERY_WORKSPACE": " ", "TF_WORKSPACE": "\t"}, SearchScope{SearchScopeValue{"user-org", "argument"}, SearchScopeValue{"user-workspace", "argument"}}, true},
		{"workspace argument overrides Terraform workspace", "org", "argument-workspace", map[string]string{"TF_MCP_QUERY_WORKSPACE": " ", "TF_WORKSPACE": " workspace "}, SearchScope{SearchScopeValue{"org", "argument"}, SearchScopeValue{"argument-workspace", "argument"}}, true},
		{"blank arguments use environment defaults", " \t", "\n", map[string]string{"TF_CLOUD_ORGANIZATION": "org", "TF_MCP_QUERY_WORKSPACE": "query", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE"}}, true},
		{"organization override preserves workspace default", "user-org", "", map[string]string{"TF_CLOUD_ORGANIZATION": "env-org", "TF_MCP_QUERY_WORKSPACE": "query"}, SearchScope{SearchScopeValue{"user-org", "argument"}, SearchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE"}}, true},
		{"workspace override preserves organization default", "", "user-workspace", map[string]string{"TF_CLOUD_ORGANIZATION": "env-org", "TF_MCP_QUERY_WORKSPACE": "query"}, SearchScope{SearchScopeValue{"env-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"user-workspace", "argument"}}, true},
		{"MCP workspace is accepted", "org", "", map[string]string{"TF_MCP_QUERY_WORKSPACE": " query ", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "argument"}, SearchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE"}}, true},
		{"Terraform defaults are used directly", "", "", map[string]string{"TF_CLOUD_ORGANIZATION": " org ", "TF_WORKSPACE": " workspace "}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"workspace", "TF_WORKSPACE"}}, true},
		{"Terraform organization with MCP workspace", "", "", map[string]string{"TF_CLOUD_ORGANIZATION": "org", "TF_MCP_QUERY_WORKSPACE": "query", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE"}}, true},
		{"missing organization with workspace argument", "", "workspace", map[string]string{"TF_CLOUD_ORGANIZATION": " "}, SearchScope{Workspace: SearchScopeValue{"workspace", "argument"}}, false},
		{"whitespace values are missing", " \t", "\n", map[string]string{"TF_CLOUD_ORGANIZATION": " ", "TF_MCP_QUERY_WORKSPACE": " ", "TF_WORKSPACE": "\n"}, SearchScope{}, false},
		{"import workspace never defaults query workspace", "org", "", map[string]string{"TF_MCP_IMPORT_WORKSPACE": "import"}, SearchScope{Organization: SearchScopeValue{"org", "argument"}}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE"} {
				t.Setenv(name, "")
			}
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			got := ResolveSearchScope(test.organization, test.workspace)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.ready, got.Ready())
		})
	}
}

func TestSearchToolsRequireResolvedScopeBeforeAPIRequests(t *testing.T) {
	for _, tool := range []server.ServerTool{ProviderListSchemaList(silentLogger()), ExecuteQuery(silentLogger())} {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			for _, test := range []struct {
				name, org, ws, mcpWS, status string
				missing                      []string
			}{
				{"missing context", "", "", "", "input_required", []string{"organization_name", "workspace_name"}},
				{"missing organization with Terraform workspace", "", "default-workspace", "", "input_required", []string{"organization_name"}},
				{"missing workspace with Terraform organization", "default-org", "", "", "input_required", []string{"workspace_name"}},
				{"missing organization with MCP workspace", "", "", "query-workspace", "input_required", []string{"organization_name"}},
			} {
				t.Run(test.name, func(t *testing.T) {
					t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
					t.Setenv("TF_WORKSPACE", test.ws)
					t.Setenv("TF_MCP_QUERY_WORKSPACE", test.mcpWS)
					// No client/session: unresolved scope must return before acquiring one.
					result, err := tool.Handler(context.Background(), mcp.CallToolRequest{})
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.IsError)
					text, ok := mcp.AsTextContent(result.Content[0])
					require.True(t, ok)
					var payload struct {
						Status  string      `json:"status"`
						Message string      `json:"message"`
						Scope   SearchScope `json:"scope"`
						Missing []string    `json:"missing_inputs"`
					}
					require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
					assert.Equal(t, test.status, payload.Status)
					assert.Equal(t, ResolveSearchScope("", ""), payload.Scope)
					assert.NotContains(t, text.Text, "requires_confirmation")
					assert.Equal(t, test.missing, payload.Missing)
					assert.Contains(t, payload.Message, "ask the user")
				})
			}
		})
	}
}

func TestSearchScopeInputRequestMessage(t *testing.T) {
	const organizationMessage = "organization_name is missing (TF_CLOUD_ORGANIZATION is unset); ask the user for it."
	const workspaceMessage = "workspace_name is missing (TF_MCP_QUERY_WORKSPACE and TF_WORKSPACE are unset); ask the user for it."
	for _, test := range []struct {
		name    string
		scope   SearchScope
		message string
		missing []string
	}{
		{"missing organization", SearchScope{Workspace: SearchScopeValue{Value: "workspace"}}, organizationMessage, []string{"organization_name"}},
		{"missing workspace", SearchScope{Organization: SearchScopeValue{Value: "org"}}, workspaceMessage, []string{"workspace_name"}},
		{"both missing", SearchScope{}, organizationMessage + " " + workspaceMessage, []string{"organization_name", "workspace_name"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload struct {
				Status  string   `json:"status"`
				Message string   `json:"message"`
				Missing []string `json:"missing_inputs"`
			}
			require.NoError(t, json.Unmarshal([]byte(test.scope.InputRequest()), &payload))
			assert.Equal(t, "input_required", payload.Status)
			assert.Equal(t, test.message, payload.Message)
			assert.Equal(t, test.missing, payload.Missing)
		})
	}
}

func TestSearchScopeDescriptionsRequireSourceConfirmationAndSeparateImportTarget(t *testing.T) {
	assert.Contains(t, QueryOrganizationDescription, "Confirm the resolved source")
	assert.Contains(t, QueryWorkspaceDescription, "Confirm the resolved source")
	assert.Contains(t, QueryWorkspaceDescription, "import target is selected and confirmed separately")
	assert.NotContains(t, QueryOrganizationDescription, "without confirmation")
	assert.NotContains(t, QueryWorkspaceDescription, "without confirmation")
}

func TestExecuteQueryUsesEnvironmentScopeDirectly(t *testing.T) {
	for _, workspaceEnv := range []string{"TF_WORKSPACE", "TF_MCP_QUERY_WORKSPACE"} {
		t.Run(workspaceEnv, func(t *testing.T) {
			t.Setenv("TF_CLOUD_ORGANIZATION", "org")
			t.Setenv("TF_WORKSPACE", "")
			t.Setenv("TF_MCP_QUERY_WORKSPACE", "")
			t.Setenv(workspaceEnv, "workspace")
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"query_configuration": "{"}
			result, err := ExecuteQuery(silentLogger()).Handler(context.Background(), request)
			require.NoError(t, err)
			require.True(t, result.IsError)
			text, ok := mcp.AsTextContent(result.Content[0])
			require.True(t, ok)
			// Resolved environment scope must reach validation, not request user input.
			assert.Contains(t, text.Text, "invalid query_configuration")
		})
	}
}
