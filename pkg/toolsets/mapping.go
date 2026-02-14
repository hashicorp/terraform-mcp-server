// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

import "strings"

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

	// Private Registry tools (TFE/TFC private registry)
	"search_private_modules":       RegistryPrivate,
	"get_private_module_details":   RegistryPrivate,
	"search_private_providers":     RegistryPrivate,
	"get_private_provider_details": RegistryPrivate,

	// Terraform tools (TFE/TFC workspaces, runs, variables, etc.)
	"list_terraform_orgs":                 Terraform,
	"list_terraform_projects":             Terraform,
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
}

// GetToolsetForTool returns the toolset name for a given tool name
func GetToolsetForTool(toolName string) (string, bool) {
	toolset, exists := ToolToToolset[toolName]
	return toolset, exists
}

// GetAllValidToolNames returns a set of all valid tool names
func GetAllValidToolNames() map[string]bool {
	validTools := make(map[string]bool)
	for toolName := range ToolToToolset {
		validTools[toolName] = true
	}
	return validTools
}

// ParseIndividualTools parses and validates individual tool names
// Returns the validated tool names and any invalid ones
func ParseIndividualTools(toolNames []string) ([]string, []string) {
	validToolNames := GetAllValidToolNames()
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

// EnableIndividualTools creates a toolset list for individual tool filtering mode
// The returned list includes an internal marker plus the specified tool names
func EnableIndividualTools(toolNames []string) []string {
	result := make([]string, 0, len(toolNames)+1)
	result = append(result, individualToolsMarker)
	result = append(result, toolNames...)
	return result
}

// IsToolEnabled checks if a tool is enabled based on the enabled toolsets
func IsToolEnabled(toolName string, enabledToolsets []string) bool {
	if ContainsToolset(enabledToolsets, All) {
		return true
	}

	// Check if we're in individual tool mode
	if ContainsToolset(enabledToolsets, individualToolsMarker) {
		// In individual tool mode, check if this specific tool is in the list
		return ContainsToolset(enabledToolsets, toolName)
	}

	// Look up which toolset this tool belongs to
	toolset, exists := GetToolsetForTool(toolName)
	if !exists {
		return false
	}

	// Check if the tool's toolset is enabled
	return ContainsToolset(enabledToolsets, toolset)
}
