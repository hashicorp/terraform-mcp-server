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
		name         string
		organization string
		workspace    string
		configure    func(*testing.T)
		want         searchScope
		ready        bool
	}{
		{
			name:         "explicit arguments override environment and are trimmed",
			organization: " user-org ", workspace: " user-workspace ",
			configure: func(t *testing.T) {
				t.Setenv("TF_CLOUD_ORGANIZATION", "env-org")
				t.Setenv("TF_MCP_QUERY_WORKSPACE", "env-workspace")
			},
			want: searchScope{searchScopeValue{"user-org", "argument", false}, searchScopeValue{"user-workspace", "argument", false}}, ready: true,
		},
		{
			name:         "MCP workspace is accepted",
			organization: "org",
			configure: func(t *testing.T) {
				t.Setenv("TF_MCP_QUERY_WORKSPACE", " query ")
				t.Setenv("TF_WORKSPACE", "fallback")
			},
			want: searchScope{searchScopeValue{"org", "argument", false}, searchScopeValue{"query", "TF_MCP_QUERY_WORKSPACE", false}}, ready: true,
		},
		{
			name: "Terraform defaults require confirmation",
			configure: func(t *testing.T) {
				t.Setenv("TF_CLOUD_ORGANIZATION", " org ")
				t.Setenv("TF_WORKSPACE", " workspace ")
			},
			want: searchScope{searchScopeValue{"org", "TF_CLOUD_ORGANIZATION", true}, searchScopeValue{"workspace", "TF_WORKSPACE", true}},
		},
		{
			name:      "organization alias is fallback",
			workspace: "workspace",
			configure: func(t *testing.T) {
				t.Setenv("TF_CLOUD_ORGANIZATION", " ")
				t.Setenv("TFC_ORGANIZATION", " alias-org ")
			},
			want: searchScope{searchScopeValue{"alias-org", "TFC_ORGANIZATION", true}, searchScopeValue{"workspace", "argument", false}},
		},
		{
			name:         "whitespace values are missing",
			organization: " \t", workspace: "\n",
			configure: func(t *testing.T) {
				t.Setenv("TF_CLOUD_ORGANIZATION", " ")
				t.Setenv("TFC_ORGANIZATION", "\t")
				t.Setenv("TF_MCP_QUERY_WORKSPACE", " ")
				t.Setenv("TF_WORKSPACE", "\n")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TFC_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE"} {
				t.Setenv(name, "")
			}
			test.configure(t)
			request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
				"organization_name": test.organization,
				"workspace_name":    test.workspace,
			}}}
			got := resolveSearchScope(request)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.ready, got.Ready())
		})
	}
}

func TestSearchToolsRequireResolvedScopeBeforeAPIRequests(t *testing.T) {
	for _, factory := range []func() server.ServerTool{
		func() server.ServerTool { return ProviderListSchemaList(silentLogger()) },
		func() server.ServerTool { return ExecuteQuery(silentLogger()) },
	} {
		tool := factory()
		t.Run(tool.Tool.Name, func(t *testing.T) {
			for _, test := range []struct {
				name   string
				org    string
				ws     string
				mcpWS  string
				status string
			}{
				{"missing context", "", "", "", "input_required"},
				{"Terraform candidates", "default-org", "default-workspace", "", "confirmation_required"},
				{"partial Terraform context", "", "default-workspace", "", "confirmation_required"},
				{"partial missing context", "default-org", "", "", "confirmation_required"},
				{"Terraform organization with MCP workspace", "default-org", "", "query-workspace", "confirmation_required"},
			} {
				t.Run(test.name, func(t *testing.T) {
					t.Setenv("TF_CLOUD_ORGANIZATION", test.org)
					t.Setenv("TFC_ORGANIZATION", "")
					t.Setenv("TF_WORKSPACE", test.ws)
					t.Setenv("TF_MCP_QUERY_WORKSPACE", test.mcpWS)
					// No session/client is supplied. Scope must be checked before
					// even attempting to acquire a client or submit a query.
					result, err := tool.Handler(context.Background(), mcp.CallToolRequest{})
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.IsError, "scope clarification is a normal response, not a tool failure")
					text, ok := mcp.AsTextContent(result.Content[0])
					require.True(t, ok)
					var payload map[string]any
					require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
					assert.Equal(t, test.status, payload["status"])
					assert.Contains(t, payload["message"], "ask the user")
				})
			}
		})
	}
}
