// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"testing"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
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

func TestResolveImportScope(t *testing.T) {
	for _, test := range []struct {
		name                    string
		organization, workspace string
		env                     map[string]string
		want                    SearchScope
		ready                   bool
	}{
		{"explicit target overrides defaults", " user-org ", " user-target ", map[string]string{"TF_CLOUD_ORGANIZATION": "env-org", "TF_MCP_IMPORT_WORKSPACE": "env-target", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"user-org", "argument"}, SearchScopeValue{"user-target", "argument"}}, true},
		{"import workspace precedes Terraform workspace", "", "", map[string]string{"TF_CLOUD_ORGANIZATION": "org", "TF_MCP_IMPORT_WORKSPACE": "target", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"target", "TF_MCP_IMPORT_WORKSPACE"}}, true},
		{"Terraform workspace is import fallback", "", "", map[string]string{"TF_CLOUD_ORGANIZATION": "org", "TF_WORKSPACE": "fallback"}, SearchScope{SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"fallback", "TF_WORKSPACE"}}, true},
		{"query workspace never defaults import target", "org", "", map[string]string{"TF_MCP_QUERY_WORKSPACE": "query"}, SearchScope{Organization: SearchScopeValue{"org", "argument"}}, false},
		{"missing target scope", "", "", nil, SearchScope{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE", "TF_WORKSPACE"} {
				t.Setenv(name, "")
			}
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			got := ResolveImportScope(test.organization, test.workspace)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.ready, got.Ready())
		})
	}
}

type scopeElicitationSession struct {
	result  *mcp.ElicitationResult
	request mcp.ElicitationRequest
}

func (*scopeElicitationSession) Initialize()                                         {}
func (*scopeElicitationSession) Initialized() bool                                   { return true }
func (*scopeElicitationSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (*scopeElicitationSession) SessionID() string                                   { return "scope-test" }
func (s *scopeElicitationSession) RequestElicitation(_ context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	s.request = request
	return s.result, nil
}

func TestSearchScopeNeedsInteraction(t *testing.T) {
	assert.False(t, (SearchScope{Organization: SearchScopeValue{"org", "argument"}, Workspace: SearchScopeValue{"ws", "argument"}}).NeedsInteraction())
	assert.True(t, (SearchScope{Organization: SearchScopeValue{"org", "TF_CLOUD_ORGANIZATION"}, Workspace: SearchScopeValue{"ws", "argument"}}).NeedsInteraction())
	assert.True(t, (SearchScope{}).NeedsInteraction())
}

func TestAcceptedSearchScope(t *testing.T) {
	scope, err := AcceptedSearchScope(map[string]any{ScopeOrganizationInput: " org ", ScopeWorkspaceInput: " ws "})
	require.NoError(t, err)
	assert.Equal(t, SearchScope{SearchScopeValue{"org", "user_input"}, SearchScopeValue{"ws", "user_input"}}, scope)

	_, err = AcceptedSearchScope(map[string]any{ScopeOrganizationInput: "org"})
	require.ErrorContains(t, err, ScopeWorkspaceInput)
}

func TestMark3LabsScopeUsesNativeElicitation(t *testing.T) {
	proposed := SearchScope{SearchScopeValue{"default-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"default-ws", "TF_MCP_QUERY_WORKSPACE"}}
	session := &scopeElicitationSession{result: &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{
		Action:  mcp.ElicitationResponseActionAccept,
		Content: map[string]any{ScopeOrganizationInput: "confirmed-org", ScopeWorkspaceInput: "confirmed-ws"},
	}}}
	mcpServer := new(server.MCPServer)
	ctx := mcpServer.WithContext(context.Background(), session)

	scope, result := resolveMark3LabsScope(ctx, mcpServer, proposed, QuerySourceScope)

	require.Nil(t, result)
	assert.Equal(t, "confirmed-org", scope.Organization.Value)
	assert.Equal(t, "confirmed-ws", scope.Workspace.Value)
	assert.Contains(t, session.request.Params.Message, `terraform_org_name="default-org"`)
	properties := session.request.Params.RequestedSchema.(map[string]any)["properties"].(map[string]any)
	assert.Equal(t, "default-org", properties[ScopeOrganizationInput].(map[string]any)["default"])
}

func TestMark3LabsScopeFailsClosedWithoutElicitation(t *testing.T) {
	for _, tool := range []server.ServerTool{ProviderListSchemaList(silentLogger()), ExecuteQuery(silentLogger())} {
		result, err := tool.Handler(context.Background(), mcp.CallToolRequest{})
		require.NoError(t, err)
		require.True(t, result.IsError)
		text, ok := mcp.AsTextContent(result.Content[0])
		require.True(t, ok)
		assert.Contains(t, text.Text, "explicit terraform_org_name and workspace_name")
	}
}

func TestMark3LabsScopeRejectsResolvedOrganizationBeforeElicitation(t *testing.T) {
	session := &scopeElicitationSession{result: &mcp.ElicitationResult{}}
	mcpServer := new(server.MCPServer)
	ctx := mcpServer.WithContext(context.Background(), session)
	ctx = terraformclient.WithOrganizationAllowlist(ctx, []string{"allowed-org"})
	proposed := SearchScope{SearchScopeValue{"blocked-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"workspace", "TF_WORKSPACE"}}

	_, result := resolveMark3LabsScope(ctx, mcpServer, proposed, QuerySourceScope)

	require.True(t, result.IsError)
	assert.Empty(t, session.request.Params.Message, "a rejected default must not be disclosed through elicitation")
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.NotContains(t, text.Text, "blocked-org")
}

func TestMark3LabsScopeRechecksAcceptedOrganization(t *testing.T) {
	session := &scopeElicitationSession{result: &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{
		Action:  mcp.ElicitationResponseActionAccept,
		Content: map[string]any{ScopeOrganizationInput: "blocked-org", ScopeWorkspaceInput: "workspace"},
	}}}
	mcpServer := new(server.MCPServer)
	ctx := mcpServer.WithContext(context.Background(), session)
	ctx = terraformclient.WithOrganizationAllowlist(ctx, []string{"allowed-org"})
	proposed := SearchScope{SearchScopeValue{"allowed-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"workspace", "TF_WORKSPACE"}}

	_, result := resolveMark3LabsScope(ctx, mcpServer, proposed, QuerySourceScope)

	require.True(t, result.IsError)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.NotContains(t, text.Text, "blocked-org")
}

func TestMark3LabsScopeDeclineAndCancelStopNormally(t *testing.T) {
	for _, action := range []mcp.ElicitationResponseAction{mcp.ElicitationResponseActionDecline, mcp.ElicitationResponseActionCancel} {
		t.Run(string(action), func(t *testing.T) {
			session := &scopeElicitationSession{result: &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: action}}}
			mcpServer := new(server.MCPServer)
			ctx := mcpServer.WithContext(context.Background(), session)
			_, result := resolveMark3LabsScope(ctx, mcpServer, SearchScope{}, ImportTargetScope)
			require.NotNil(t, result)
			require.False(t, result.IsError)
			assert.Contains(t, fmt.Sprint(result.Content), "no scoped API request was made")
		})
	}
}

func TestSearchScopeDescriptionsUseCanonicalOrganizationAndIndependentTargets(t *testing.T) {
	assert.Contains(t, QueryOrganizationDescription, ScopeOrganizationInput)
	assert.Contains(t, QueryWorkspaceDescription, "native MCP user input")
	assert.Contains(t, QueryWorkspaceDescription, "Import scope is resolved independently")
	assert.Contains(t, ImportOrganizationDescription, "independent")
	assert.Contains(t, ImportWorkspaceDescription, "TF_MCP_IMPORT_WORKSPACE")
	assert.Contains(t, ImportWorkspaceDescription, "TF_MCP_QUERY_WORKSPACE is never")
}
