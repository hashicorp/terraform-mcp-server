// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"encoding/json"
	"os"
	"strings"
)

type SearchScopeValue struct {
	Value                string `json:"value"`
	Source               string `json:"source,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

// SearchScope carries context and confirmation metadata for either MCP transport.
type SearchScope struct {
	Organization SearchScopeValue `json:"organization_name"`
	Workspace    SearchScopeValue `json:"workspace_name"`
}

// ResolveSearchScope preserves explicit inputs and treats Terraform defaults as candidates.
func ResolveSearchScope(organization, workspace string) SearchScope {
	return SearchScope{
		Organization: resolveSearchScopeValue(organization, "", "TF_CLOUD_ORGANIZATION", "TFC_ORGANIZATION"),
		Workspace:    resolveSearchScopeValue(workspace, "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE"),
	}
}

func resolveSearchScopeValue(explicit, mcpEnv string, defaults ...string) SearchScopeValue {
	if value := strings.TrimSpace(explicit); value != "" {
		return SearchScopeValue{Value: value, Source: "argument"}
	}
	if mcpEnv != "" {
		if value := strings.TrimSpace(os.Getenv(mcpEnv)); value != "" {
			return SearchScopeValue{Value: value, Source: mcpEnv}
		}
	}
	for _, name := range defaults {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return SearchScopeValue{Value: value, Source: name, RequiresConfirmation: true}
		}
	}
	return SearchScopeValue{}
}

func (s SearchScope) Ready() bool {
	return s.Organization.Value != "" && s.Workspace.Value != "" &&
		!s.Organization.RequiresConfirmation && !s.Workspace.RequiresConfirmation
}

// InputRequest exposes only organization/workspace identifiers, never credentials.
func (s SearchScope) InputRequest() string {
	missing := []string{}
	if s.Organization.Value == "" {
		missing = append(missing, "organization_name")
	}
	if s.Workspace.Value == "" {
		missing = append(missing, "workspace_name")
	}
	status := "input_required"
	message := "organization_name and workspace_name are required; ask the user for the missing inputs, then retry with explicit arguments."
	if s.Organization.RequiresConfirmation || s.Workspace.RequiresConfirmation {
		status = "confirmation_required"
		message = "Terraform defaults are candidates only; ask the user whether to use these defaults or supply their own. Ask for any missing inputs too. Wait for their response, then retry with accepted or replacement values as explicit arguments. Do not make scoped API requests before confirmation."
	}
	payload, _ := json.Marshal(struct {
		Status  string      `json:"status"`
		Message string      `json:"message"`
		Scope   SearchScope `json:"scope"`
		Missing []string    `json:"missing_inputs"`
	}{status, message, s, missing})
	return string(payload)
}

const QueryOrganizationDescription = "HCP Terraform organization containing the query workspace. Explicit input overrides environment values. TF_CLOUD_ORGANIZATION, then TFC_ORGANIZATION, are candidates only: ask the user to confirm or replace them before passing the value. If absent, ask the user for organization_name."

const QueryWorkspaceDescription = "HCP Terraform workspace for queries. Explicit input overrides TF_MCP_QUERY_WORKSPACE. If neither is set, TF_WORKSPACE is a candidate only: ask the user to confirm or replace it before passing the value. If absent, ask the user for workspace_name. TF_MCP_IMPORT_WORKSPACE is a separate import target, not a query default."
