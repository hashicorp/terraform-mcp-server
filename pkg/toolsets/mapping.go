// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

import (
	"strings"
	"sync"
)

// toolsetIndex is a memoized "tool name -> toolset name" lookup, built once
// from AllTools (registry.go) instead of hand-maintaining a second map
var toolsetIndex = sync.OnceValue(func() map[string]string {
	index := make(map[string]string, len(AllTools))
	for _, td := range AllTools {
		index[td.Name] = td.Toolset
	}
	return index
})

var ToolToToolset = map[string]string{
	// Public Registry tools (providers, modules, policies)
	"search_providers":            Registry,
	"get_provider_details":        Registry,
	"get_latest_provider_version": Registry,
	"get_provider_capabilities":   Registry,
	"search_modules":              Registry,
	"get_module_details":          Registry,
	"get_latest_module_version":   Registry,
	"search_policies":             Registry,
	"get_policy_details":          Registry,

	// Search tools (HCP Terraform no-code search)
	"generate_query_configuration": Search,
	"provider_list_schema_list":    Search,
	"execute_query":                Search,
	"get_query_status":             Search,
	"get_query_summary":            Search,

	// Private Registry tools (TFE/TFC private registry)
	"search_private_modules":       RegistryPrivate,
	"get_private_module_details":   RegistryPrivate,
	"search_private_providers":     RegistryPrivate,
	"get_private_provider_details": RegistryPrivate,

	// Terraform tools (TFE/TFC workspaces, runs, variables, etc.)
	"list_terraform_orgs":                 Terraform,
	"list_terraform_projects":             Terraform,
	"create_project":                      Terraform,
	"delete_project":                      Terraform,
	"list_workspaces":                     Terraform,
	"get_workspace_details":               Terraform,
	"create_workspace":                    Terraform,
	"create_no_code_workspace":            Terraform,
	"update_workspace":                    Terraform,
	"delete_workspace_safely":             Terraform,
	"list_runs":                           Terraform,
	"get_run_details":                     Terraform,
	"get_plan_details":                    Terraform,
	"get_plan_logs":                       Terraform,
	"get_plan_json_output":                Terraform,
	"get_apply_details":                   Terraform,
	"get_apply_logs":                      Terraform,
	"get_sentinel_mock":                   Terraform,
	"create_run":                          Terraform,
	"action_run":                          Terraform,
	"list_workspace_variables":            Terraform,
	"create_workspace_variable":           Terraform,
	"update_workspace_variable":           Terraform,
	"list_variable_sets":                  Terraform,
	"create_variable_set":                 Terraform,
	"create_variable_in_variable_set":     Terraform,
	"delete_variable_in_variable_set":     Terraform,
	"attach_variable_set_to_workspaces":   Terraform,
	"detach_variable_set_from_workspaces": Terraform,
	"create_workspace_tags":               Terraform,
	"read_workspace_tags":                 Terraform,
	"attach_policy_set_to_workspaces":     Terraform,
	"get_token_permissions":               Terraform,
	"list_stacks":                         Terraform,
	"get_stack_details":                   Terraform,
	"list_workspace_policy_sets":          Terraform,
	"force_unlock_workspace":              Terraform,
	"list_state_versions":                 Terraform,
	"get_state_version":                   Terraform,
	"get_run_comments":                    Terraform,
}

// GetToolsetForTool returns the toolset name for a given tool name
func GetToolsetForTool(toolName string) (string, bool) {
	toolset, exists := toolsetIndex()[toolName]
	return toolset, exists
}

// KnownToolNames returns every tool name registered in toolsetIndex
func KnownToolNames() map[string]bool {
	index := toolsetIndex()
	validTools := make(map[string]bool, len(index))
	for toolName := range index {
		validTools[toolName] = true
	}
	return validTools
}

// ParseIndividualTools parses and validates individual tool names
// Returns the validated tool names and any invalid ones
func ParseIndividualTools(toolNames []string) ([]string, []string) {
	validToolNames := KnownToolNames()
	seen := make(map[string]bool)
	valid := make([]string, 0, len(toolNames))
	invalid := make([]string, 0)

	for _, name := range toolNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			if validToolNames[trimmed] {
				valid = append(valid, trimmed)
			} else {
				invalid = append(invalid, trimmed)
			}
		}
	}

	return valid, invalid
}
