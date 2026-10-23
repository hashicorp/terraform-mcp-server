// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"testing"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
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

func TestMark3LabsScopeReturnsCompletedInputRequest(t *testing.T) {
	for _, test := range []struct {
		name     string
		role     SearchScopeRole
		proposed SearchScope
		missing  []string
	}{
		{
			name:     "complete query defaults require explicit retry",
			role:     QuerySourceScope,
			proposed: SearchScope{SearchScopeValue{"default-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"default-query", "TF_MCP_QUERY_WORKSPACE"}},
			missing:  []string{},
		},
		{
			name:     "partial query defaults identify missing workspace",
			role:     QuerySourceScope,
			proposed: SearchScope{Organization: SearchScopeValue{"default-org", "TF_CLOUD_ORGANIZATION"}},
			missing:  []string{ScopeWorkspaceInput},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope, result := resolveMark3LabsScope(context.Background(), test.proposed, test.role)
			require.Equal(t, SearchScope{}, scope)
			require.NotNil(t, result)
			require.False(t, result.IsError)
			text, ok := mcp.AsTextContent(result.Content[0])
			require.True(t, ok)

			var payload mark3LabsScopeInput
			require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
			assert.Equal(t, "input_required", payload.Status)
			assert.Contains(t, payload.Message, string(test.role))
			assert.Contains(t, payload.Message, "explicit terraform_org_name and workspace_name")
			assert.Equal(t, test.proposed.Organization.Value, payload.ProposedScope.OrganizationName)
			assert.Equal(t, test.proposed.Workspace.Value, payload.ProposedScope.WorkspaceName)
			assert.Equal(t, test.missing, payload.MissingInputs)
		})
	}
}

func TestMark3LabsSearchToolsRequestScopeBeforeClientUse(t *testing.T) {
	for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE"} {
		t.Setenv(name, "")
	}
	for _, tool := range []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{name: "provider_list_schema_list", handler: ProviderListSchemaList(silentLogger()).Handler},
	} {
		t.Run(tool.name, func(t *testing.T) {
			result, err := tool.handler(context.Background(), mcp.CallToolRequest{})
			require.NoError(t, err)
			require.False(t, result.IsError)
			text, ok := mcp.AsTextContent(result.Content[0])
			require.True(t, ok)
			assert.Contains(t, text.Text, `"status":"input_required"`)
			assert.Contains(t, text.Text, `"missing_inputs":["terraform_org_name","workspace_name"]`)
		})
	}
}

func TestMark3LabsProviderToolReturnsConfiguredProposalBeforeClientUse(t *testing.T) {
	t.Setenv("TF_CLOUD_ORGANIZATION", "configured-org")
	t.Setenv("TF_MCP_QUERY_WORKSPACE", "configured-query")
	t.Setenv("TF_WORKSPACE", "fallback")

	result, err := ProviderListSchemaList(silentLogger()).Handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)

	var payload mark3LabsScopeInput
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	assert.Equal(t, "configured-org", payload.ProposedScope.OrganizationName)
	assert.Equal(t, "configured-query", payload.ProposedScope.WorkspaceName)
	assert.Empty(t, payload.MissingInputs)
}

func TestMark3LabsScopeRejectsResolvedOrganizationBeforeDisclosure(t *testing.T) {
	ctx := context.Background()
	ctx = terraformclient.WithOrganizationAllowlist(ctx, []string{"allowed-org"})
	proposed := SearchScope{SearchScopeValue{"blocked-org", "TF_CLOUD_ORGANIZATION"}, SearchScopeValue{"workspace", "TF_WORKSPACE"}}

	_, result := resolveMark3LabsScope(ctx, proposed, QuerySourceScope)

	require.True(t, result.IsError)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.NotContains(t, text.Text, "blocked-org")
}

func TestMark3LabsScopeChecksExplicitOrganization(t *testing.T) {
	ctx := context.Background()
	ctx = terraformclient.WithOrganizationAllowlist(ctx, []string{"allowed-org"})
	explicit := SearchScope{SearchScopeValue{"blocked-org", "argument"}, SearchScopeValue{"workspace", "argument"}}

	_, result := resolveMark3LabsScope(ctx, explicit, QuerySourceScope)

	require.True(t, result.IsError)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.NotContains(t, text.Text, "blocked-org")
}

func TestSearchScopeDescriptionsUseCanonicalOrganizationAndIndependentTargets(t *testing.T) {
	assert.Contains(t, QueryOrganizationDescription, ScopeOrganizationInput)
	assert.Contains(t, QueryWorkspaceDescription, "requests confirmation")
	assert.Contains(t, QueryWorkspaceDescription, "Import scope is resolved independently")
}
