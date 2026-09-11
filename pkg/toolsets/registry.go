// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

// ToolDef is the single source of truth for a tool's registration metadata:
// which toolset it belongs to, and what runtime gates control it.
type ToolDef struct {
	Name          string
	Toolset       string
	RequiresTFE   bool // needs an authenticated TFE/TFC session before it's registered
	RequiresTFOps bool // additionally gated behind ENABLE_TF_OPERATIONS=true
}

// AllTools is the full list of tools known to the server. Both the
// mark3labs and official-SDK registration code should loop over this
// instead of hardcoding their own per-tool if-chain.
var AllTools = []ToolDef{
	// Public Registry tools (providers, modules, policies) — no TFE session needed
	{Name: "search_providers", Toolset: Registry},
	{Name: "get_provider_details", Toolset: Registry},
	{Name: "get_latest_provider_version", Toolset: Registry},
	{Name: "get_provider_capabilities", Toolset: Registry},
	{Name: "search_modules", Toolset: Registry},
	{Name: "get_module_details", Toolset: Registry},
	{Name: "get_latest_module_version", Toolset: Registry},
	{Name: "search_policies", Toolset: Registry},
	{Name: "get_policy_details", Toolset: Registry},

	// Private Registry tools (TFE/TFC private registry)
	{Name: "search_private_modules", Toolset: RegistryPrivate, RequiresTFE: true},
	{Name: "get_private_module_details", Toolset: RegistryPrivate, RequiresTFE: true},
	{Name: "search_private_providers", Toolset: RegistryPrivate, RequiresTFE: true},
	{Name: "get_private_provider_details", Toolset: RegistryPrivate, RequiresTFE: true},

	// Terraform - User
	{Name: "whoami", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_token_permissions", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Organization
	{Name: "list_terraform_orgs", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Projects
	{Name: "list_terraform_projects", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_project", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_project", Toolset: Terraform, RequiresTFE: true},
	{Name: "delete_project", Toolset: Terraform, RequiresTFE: true, RequiresTFOps: true},

	// Terraform - Teams
	{Name: "list_teams", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_team", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_team", Toolset: Terraform, RequiresTFE: true},
	{Name: "add_team_member", Toolset: Terraform, RequiresTFE: true},
	{Name: "grant_team_access", Toolset: Terraform, RequiresTFE: true},
	{Name: "delete_team", Toolset: Terraform, RequiresTFE: true, RequiresTFOps: true},

	// Terraform - Workspaces
	{Name: "list_workspaces", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_workspace_details", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_workspace", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_no_code_workspace", Toolset: Terraform, RequiresTFE: true},
	{Name: "update_workspace", Toolset: Terraform, RequiresTFE: true},
	{Name: "delete_workspace_safely", Toolset: Terraform, RequiresTFE: true, RequiresTFOps: true},
	{Name: "force_unlock_workspace", Toolset: Terraform, RequiresTFE: true, RequiresTFOps: true},

	// Terraform - Runs and Plans
	{Name: "list_runs", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_run_details", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_run_comments", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_run", Toolset: Terraform, RequiresTFE: true}, // Handle special case
	{Name: "action_run", Toolset: Terraform, RequiresTFE: true, RequiresTFOps: true},
	{Name: "get_plan_details", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_plan_logs", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_plan_json_output", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_apply_details", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_apply_logs", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_sentinel_mock", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Workspace Variables
	{Name: "list_workspace_variables", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_workspace_variable", Toolset: Terraform, RequiresTFE: true},
	{Name: "update_workspace_variable", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Variable Sets
	{Name: "list_variable_sets", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_variable_set", Toolset: Terraform, RequiresTFE: true},
	{Name: "create_variable_in_variable_set", Toolset: Terraform, RequiresTFE: true},
	{Name: "delete_variable_in_variable_set", Toolset: Terraform, RequiresTFE: true},
	{Name: "attach_variable_set_to_workspaces", Toolset: Terraform, RequiresTFE: true},
	{Name: "detach_variable_set_from_workspaces", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Tags and Policies
	{Name: "create_workspace_tags", Toolset: Terraform, RequiresTFE: true},
	{Name: "read_workspace_tags", Toolset: Terraform, RequiresTFE: true},
	{Name: "attach_policy_set_to_workspaces", Toolset: Terraform, RequiresTFE: true},
	{Name: "list_workspace_policy_sets", Toolset: Terraform, RequiresTFE: true},

	// Terraform - Stacks
	{Name: "list_stacks", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_stack_details", Toolset: Terraform, RequiresTFE: true},

	// Terraform - State Versions
	{Name: "list_state_versions", Toolset: Terraform, RequiresTFE: true},
	{Name: "get_state_version", Toolset: Terraform, RequiresTFE: true},
}
