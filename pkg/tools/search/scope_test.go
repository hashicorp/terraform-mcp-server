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
		{"explicit arguments override environment and are trimmed", " user-org ", " user-workspace ", map[string]string{"TF_CLOUD_ORGANIZATION": "env-org", "TF_MCP_QUERY_WORKSPACE": "env-workspace"}, SearchScope{SearchScopeValue{"user-org", "argument", false}, SearchScopeValue{"user-workspace", "argument", false}}, true},
		{"MCP workspace is accepted", "org", "", map[string]string{"TF_MCP_QUERY_WORKSPACE": " query ", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "argument", false}, SearchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE", false}}, true},
		{"Terraform defaults require confirmation", "", "", map[string]string{"TF_CLOUD_ORGANIZATION": " org ", "TF_WORKSPACE": " workspace "}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION", true}, SearchScopeValue{"workspace", "TF_WORKSPACE", true}}, false},
		{"organization alias is fallback", "", "workspace", map[string]string{"TF_CLOUD_ORGANIZATION": " ", "TFC_ORGANIZATION": " alias-org "}, SearchScope{SearchScopeValue{"alias-org", "TFC_ORGANIZATION", true}, SearchScopeValue{"workspace", "argument", false}}, false},
		{"primary organization wins over alias", "", "workspace", map[string]string{"TF_CLOUD_ORGANIZATION": "primary", "TFC_ORGANIZATION": "alias"}, SearchScope{SearchScopeValue{"primary", "TF_CLOUD_ORGANIZATION", true}, SearchScopeValue{"workspace", "argument", false}}, false},
		{"whitespace values are missing", " \t", "\n", map[string]string{"TF_CLOUD_ORGANIZATION": " ", "TFC_ORGANIZATION": "\t", "TF_MCP_QUERY_WORKSPACE": " ", "TF_WORKSPACE": "\n"}, SearchScope{}, false},
		{"import workspace never defaults query workspace", "org", "", map[string]string{"TF_MCP_IMPORT_WORKSPACE": "import"}, SearchScope{Organization: SearchScopeValue{"org", "argument", false}}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TFC_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE"} {
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
				{"Terraform candidates", "default-org", "default-workspace", "", "confirmation_required", []string{}},
				{"partial Terraform context", "", "default-workspace", "", "confirmation_required", []string{"organization_name"}},
				{"partial missing context", "default-org", "", "", "confirmation_required", []string{"workspace_name"}},
				{"Terraform organization with MCP workspace", "default-org", "", "query-workspace", "confirmation_required", []string{}},
			} {
				t.Run(test.name, func(t *testing.T) {
					t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
					t.Setenv("TFC_ORGANIZATION", "")
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
						Status  string   `json:"status"`
						Message string   `json:"message"`
						Missing []string `json:"missing_inputs"`
					}
					require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
					assert.Equal(t, test.status, payload.Status)
					assert.Equal(t, test.missing, payload.Missing)
					assert.Contains(t, payload.Message, "ask the user")
				})
			}
		})
	}
}
