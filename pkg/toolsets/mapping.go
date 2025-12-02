// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

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
}

// GetToolsetForTool returns the toolset name for a given tool name
func GetToolsetForTool(toolName string) (string, bool) {
	toolset, exists := ToolToToolset[toolName]
	return toolset, exists
}

// IsToolEnabled checks if a tool is enabled based on the enabled toolsets
func IsToolEnabled(toolName string, enabledToolsets []string) bool {
	if ContainsToolset(enabledToolsets, All) {
		return true
	}

	// Look up which toolset this tool belongs to
	toolset, exists := GetToolsetForTool(toolName)
	if !exists {
		return false
	}

	// Check if the tool's toolset is enabled
	return ContainsToolset(enabledToolsets, toolset)
}
