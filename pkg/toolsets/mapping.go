// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

// ToolToToolset maps tool names to their toolsets
var ToolToToolset = map[string]Toolset{
	// Public Registry tools (providers, modules, policies)
	"search_providers":            ToolsetRegistry,
	"get_provider_details":        ToolsetRegistry,
	"get_latest_provider_version": ToolsetRegistry,
	"get_provider_capabilities":   ToolsetRegistry,
	"search_modules":              ToolsetRegistry,
	"get_module_details":          ToolsetRegistry,
	"get_latest_module_version":   ToolsetRegistry,
	"search_policies":             ToolsetRegistry,
	"get_policy_details":          ToolsetRegistry,

	// Private Registry tools (TFE/TFC private registry)
	"search_private_modules":       ToolsetRegistryPrivate,
	"get_private_module_details":   ToolsetRegistryPrivate,
	"search_private_providers":     ToolsetRegistryPrivate,
	"get_private_provider_details": ToolsetRegistryPrivate,

	// Terraform tools (TFE/TFC workspaces, runs, variables, etc.)
	"list_terraform_orgs":                 ToolsetTerraform,
	"list_terraform_projects":             ToolsetTerraform,
	"list_workspaces":                     ToolsetTerraform,
	"get_workspace_details":               ToolsetTerraform,
	"create_workspace":                    ToolsetTerraform,
	"create_no_code_workspace":            ToolsetTerraform,
	"update_workspace":                    ToolsetTerraform,
	"delete_workspace_safely":             ToolsetTerraform,
	"list_runs":                           ToolsetTerraform,
	"get_run_details":                     ToolsetTerraform,
	"create_run":                          ToolsetTerraform,
	"action_run":                          ToolsetTerraform,
	"list_workspace_variables":            ToolsetTerraform,
	"create_workspace_variable":           ToolsetTerraform,
	"update_workspace_variable":           ToolsetTerraform,
	"list_variable_sets":                  ToolsetTerraform,
	"create_variable_set":                 ToolsetTerraform,
	"create_variable_in_variable_set":     ToolsetTerraform,
	"delete_variable_in_variable_set":     ToolsetTerraform,
	"attach_variable_set_to_workspaces":   ToolsetTerraform,
	"detach_variable_set_from_workspaces": ToolsetTerraform,
	"create_workspace_tags":               ToolsetTerraform,
	"read_workspace_tags":                 ToolsetTerraform,
}

// GetToolsetForTool returns the toolset for a given tool name
func GetToolsetForTool(toolName string) (Toolset, bool) {
	toolset, exists := ToolToToolset[toolName]
	return toolset, exists
}

// IsToolEnabled checks if a tool is enabled based on the enabled toolsets
func IsToolEnabled(toolName string, enabledToolsets []string) bool {
	if ContainsToolset(enabledToolsets, string(ToolsetAll)) {
		return true
	}

	// Look up which toolset this tool belongs to
	toolset, exists := GetToolsetForTool(toolName)
	if !exists {
		return false
	}

	// Check if the tool's toolset is enabled
	return ContainsToolset(enabledToolsets, string(toolset))
}
