// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"encoding/json"
	"os"
	"strings"
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

// InputRequest exposes only organization/workspace identifiers, never credentials.
func (s SearchScope) InputRequest() string {
	missing := []string{}
	messages := []string{}
	if s.Organization.Value == "" {
		missing = append(missing, "organization_name")
		messages = append(messages, "organization_name is missing (TF_CLOUD_ORGANIZATION is unset); ask the user for it.")
	}
	if s.Workspace.Value == "" {
		missing = append(missing, "workspace_name")
		messages = append(messages, "workspace_name is missing (TF_MCP_QUERY_WORKSPACE and TF_WORKSPACE are unset); ask the user for it.")
	}
	status := "input_required"
	message := strings.Join(messages, " ")
	payload, _ := json.Marshal(struct {
		Status  string      `json:"status"`
		Message string      `json:"message"`
		Scope   SearchScope `json:"scope"`
		Missing []string    `json:"missing_inputs"`
	}{status, message, s, missing})
	return string(payload)
}

const QueryOrganizationDescription = "Optional. HCP Terraform organization containing the query workspace. Omit to resolve the proposed source from TF_CLOUD_ORGANIZATION. Confirm the resolved source with the user before the query workflow. If the user specifies a different organization, pass it here to override the default. Ask only for fields the tool reports missing."

const QueryWorkspaceDescription = "Optional. HCP Terraform source workspace for queries. Omit to resolve the proposed source from TF_MCP_QUERY_WORKSPACE, then TF_WORKSPACE. Confirm the resolved source with the user before the query workflow. If the user specifies a different workspace, pass it here to override the default. Ask only for fields the tool reports missing. The import target is selected and confirmed separately, never defaulted from query scope."
