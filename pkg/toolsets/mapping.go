// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

// ToolToToolset maps tool names to their toolsets
var ToolToToolset = map[string]Toolset{
	// Provider tools
	"search_providers":            ToolsetProviders,
	"get_provider_details":        ToolsetProviders,
	"get_latest_provider_version": ToolsetProviders,
	"get_provider_capabilities":   ToolsetProviders,

	// Module tools
	"search_modules":            ToolsetModules,
	"get_module_details":        ToolsetModules,
	"get_latest_module_version": ToolsetModules,

	// Policy tools
	"search_policies":    ToolsetPolicies,
	"get_policy_details": ToolsetPolicies,

	// Workspace tools
	"list_workspaces":          ToolsetWorkspaces,
	"get_workspace_details":    ToolsetWorkspaces,
	"create_workspace":         ToolsetWorkspaces,
	"create_no_code_workspace": ToolsetWorkspaces,
	"update_workspace":         ToolsetWorkspaces,
	"delete_workspace_safely":  ToolsetWorkspaces,

	// Run tools
	"list_runs":       ToolsetRuns,
	"get_run_details": ToolsetRuns,
	"create_run":      ToolsetRuns,
	"action_run":      ToolsetRuns,

	// Organization tools
	"list_terraform_orgs": ToolsetOrganizations,

	// Project tools
	"list_terraform_projects": ToolsetProjects,

	// Variable tools
	"list_workspace_variables":  ToolsetVariables,
	"create_workspace_variable": ToolsetVariables,
	"update_workspace_variable": ToolsetVariables,

	// Variable set tools
	"list_variable_sets":                  ToolsetVariableSets,
	"create_variable_set":                 ToolsetVariableSets,
	"create_variable_in_variable_set":     ToolsetVariableSets,
	"delete_variable_in_variable_set":     ToolsetVariableSets,
	"attach_variable_set_to_workspaces":   ToolsetVariableSets,
	"detach_variable_set_from_workspaces": ToolsetVariableSets,

	// Tag tools
	"create_workspace_tags": ToolsetTags,
	"read_workspace_tags":   ToolsetTags,

	// Private registry tools
	"search_private_modules":       ToolsetPrivateRegistry,
	"get_private_module_details":   ToolsetPrivateRegistry,
	"search_private_providers":     ToolsetPrivateRegistry,
	"get_private_provider_details": ToolsetPrivateRegistry,
}

func GetToolsetForTool(toolName string) (Toolset, bool) {
	toolset, exists := ToolToToolset[toolName]
	return toolset, exists
}

// checks if a tool is enabled based on the enabled toolsets
func IsToolEnabled(toolName string, enabledToolsets []string) bool {
	if ContainsToolset(enabledToolsets, string(ToolsetAll)) {
		return true
	}

	toolset, exists := GetToolsetForTool(toolName)
	if !exists {
		return false
	}

	return ContainsToolset(enabledToolsets, string(toolset))
}
