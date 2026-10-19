// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"fmt"
	"os"
	"strings"
)

const (
	ScopeOrganizationInput = "terraform_org_name"
	ScopeWorkspaceInput    = "workspace_name"
	ScopeInputRequestID    = "search_scope"
)

type SearchScopeRole string

const (
	QuerySourceScope  SearchScopeRole = "Search source"
	ImportTargetScope SearchScopeRole = "import target"
)

type SearchScopeValue struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
}

// SearchScope carries resolved context and its sources for either MCP transport.
type SearchScope struct {
	Organization SearchScopeValue `json:"organization_name"`
	Workspace    SearchScopeValue `json:"workspace_name"`
}

// ResolveSearchScope uses explicit scope overrides, otherwise configured environment defaults.
func ResolveSearchScope(organization, workspace string) SearchScope {
	return SearchScope{
		Organization: resolveSearchScopeValue(organization, "TF_CLOUD_ORGANIZATION"),
		Workspace:    resolveSearchScopeValue(workspace, "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE"),
	}
}

// ResolveImportScope uses explicit target overrides, otherwise configured import defaults.
// Resolving a default does not replace the agent's requirement to confirm the target.
func ResolveImportScope(organization, workspace string) SearchScope {
	return SearchScope{
		Organization: resolveSearchScopeValue(organization, "TF_CLOUD_ORGANIZATION"),
		Workspace:    resolveSearchScopeValue(workspace, "TF_MCP_IMPORT_WORKSPACE", "TF_WORKSPACE"),
	}
}

func resolveSearchScopeValue(explicit string, envNames ...string) SearchScopeValue {
	if value := strings.TrimSpace(explicit); value != "" {
		return SearchScopeValue{Value: value, Source: "argument"}
	}
	for _, name := range envNames {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return SearchScopeValue{Value: value, Source: name}
		}
	}
	return SearchScopeValue{}
}

func (s SearchScope) Ready() bool {
	return s.Organization.Value != "" && s.Workspace.Value != ""
}

// NeedsInteraction reports whether the complete scope was not supplied as tool
// arguments. Environment values are proposed defaults, not authorization.
func (s SearchScope) NeedsInteraction() bool {
	return !s.Ready() || s.Organization.Source != "argument" || s.Workspace.Source != "argument"
}

// InteractionMessage exposes only the proposed organization/workspace and
// their role. It never includes credentials, addresses, or unrelated settings.
func (s SearchScope) InteractionMessage(role SearchScopeRole) string {
	organization := s.Organization.Value
	if organization == "" {
		organization = "<required>"
	}
	workspace := s.Workspace.Value
	if workspace == "" {
		workspace = "<required>"
	}
	return fmt.Sprintf("Confirm the %s organization and workspace. Proposed %s=%q and %s=%q. Supply the complete pair or decline/cancel; configured defaults are not authorization.", role, ScopeOrganizationInput, organization, ScopeWorkspaceInput, workspace)
}

// AcceptedSearchScope validates the complete values returned by native MCP
// elicitation or an input-required retry.
func AcceptedSearchScope(content map[string]any) (SearchScope, error) {
	organization, ok := content[ScopeOrganizationInput].(string)
	if !ok || strings.TrimSpace(organization) == "" {
		return SearchScope{}, fmt.Errorf("%s is required", ScopeOrganizationInput)
	}
	workspace, ok := content[ScopeWorkspaceInput].(string)
	if !ok || strings.TrimSpace(workspace) == "" {
		return SearchScope{}, fmt.Errorf("%s is required", ScopeWorkspaceInput)
	}
	return SearchScope{
		Organization: SearchScopeValue{Value: strings.TrimSpace(organization), Source: "user_input"},
		Workspace:    SearchScopeValue{Value: strings.TrimSpace(workspace), Source: "user_input"},
	}, nil
}

const QueryOrganizationDescription = "Optional explicit HCP Terraform organization containing the query workspace. The canonical terraform_org_name input overrides TF_CLOUD_ORGANIZATION. When omitted, the server proposes TF_CLOUD_ORGANIZATION through native MCP user input before scoped API access. Reuse the accepted complete source pair explicitly in later Search calls."

const QueryWorkspaceDescription = "Optional explicit HCP Terraform source workspace. This input overrides TF_MCP_QUERY_WORKSPACE, then TF_WORKSPACE. When omitted, the server proposes the first configured value through native MCP user input before scoped API access. Reuse the accepted complete source pair explicitly. Import scope is resolved independently."

const ImportOrganizationDescription = "Optional explicit confirmed target organization. The canonical terraform_org_name input overrides TF_CLOUD_ORGANIZATION. When omitted, the server proposes TF_CLOUD_ORGANIZATION through native MCP user input before scoped API access. Import-target confirmation is independent from query-source confirmation."

const ImportWorkspaceDescription = "Optional explicit confirmed import target workspace. This input overrides TF_MCP_IMPORT_WORKSPACE, then TF_WORKSPACE. When omitted, the server proposes the first configured value through native MCP user input before scoped API access. TF_MCP_QUERY_WORKSPACE is never an import default."
