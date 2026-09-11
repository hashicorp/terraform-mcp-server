// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/registry"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/tfe"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// toolFactory builds a server.ServerTool (Tool + Handler) for one tool.
type toolFactory func(logger *log.Logger) server.ServerTool

// toolFactories is the single lookup both RegisterTools (tools.go, registry
// tools) and registerTFETools (dynamic_tool.go, TFE tools) dispatch through,
// keyed by toolsets.ToolDef.Name. This replaces the copy-pasted
// `if toolsets.IsToolEnabled(...) { tool := xTools.Y(logger); AddTool(...) }`
// block that used to appear once per tool in each registration site.
//
// Two tools are intentionally NOT here — they don't fit the plain
// func(*log.Logger) server.ServerTool shape and are handled explicitly where
// they're registered, in dynamic_tool.go:
//   - create_no_code_workspace: also needs *server.MCPServer (elicitation).
//   - create_run: swaps between CreateRun/CreateRunSafe based on
//     ENABLE_TF_OPERATIONS.
var toolFactories = map[string]toolFactory{
	// Public Registry tools
	"search_providers":            registryTools.ResolveProviderDocID,
	"get_provider_details":        registryTools.GetProviderDocs,
	"get_latest_provider_version": registryTools.GetLatestProviderVersion,
	"get_provider_capabilities":   registryTools.GetProviderCapabilities,
	"search_modules":              registryTools.SearchModules,
	"get_module_details":          registryTools.ModuleDetails,
	"get_latest_module_version":   registryTools.GetLatestModuleVersion,
	"search_policies":             registryTools.SearchPolicies,
	"get_policy_details":          registryTools.PolicyDetails,

	// Private Registry tools
	"search_private_modules":       tfeTools.SearchPrivateModules,
	"get_private_module_details":   tfeTools.GetPrivateModuleDetails,
	"search_private_providers":     tfeTools.SearchPrivateProviders,
	"get_private_provider_details": tfeTools.GetPrivateProviderDetails,

	// Terraform - User
	"whoami":                tfeTools.WhoAmI,
	"get_token_permissions": tfeTools.GetTokenPermissions,

	// Terraform - Organization
	"list_terraform_orgs": tfeTools.ListTerraformOrgs,

	// Terraform - Projects
	"list_terraform_projects": tfeTools.ListTerraformProjects,
	"create_project":          tfeTools.CreateProject,
	"get_project":             tfeTools.GetProject,
	"delete_project":          tfeTools.DeleteProject,

	// Terraform - Teams
	"list_teams":        tfeTools.ListTeams,
	"get_team":          tfeTools.GetTeam,
	"create_team":       tfeTools.CreateTeam,
	"add_team_member":   tfeTools.AddTeamMember,
	"grant_team_access": tfeTools.GrantTeamAccess,
	"delete_team":       tfeTools.DeleteTeam,

	// Terraform - Workspaces (create_no_code_workspace excepted, see above)
	"list_workspaces":         tfeTools.ListWorkspaces,
	"get_workspace_details":   tfeTools.GetWorkspaceDetails,
	"create_workspace":        tfeTools.CreateWorkspace,
	"update_workspace":        tfeTools.UpdateWorkspace,
	"delete_workspace_safely": tfeTools.DeleteWorkspaceSafely,
	"force_unlock_workspace":  tfeTools.ForceUnlockWorkspace,

	// Terraform - Runs and Plans (create_run excepted, see above)
	"list_runs":            tfeTools.ListRuns,
	"get_run_details":      tfeTools.GetRunDetails,
	"get_run_comments":     tfeTools.GetRunComments,
	"action_run":           tfeTools.ActionRun,
	"get_plan_details":     tfeTools.GetPlanDetails,
	"get_plan_logs":        tfeTools.GetPlanLogs,
	"get_plan_json_output": tfeTools.GetPlanJSONOutput,
	"get_apply_details":    tfeTools.GetApplyDetails,
	"get_apply_logs":       tfeTools.GetApplyLogs,
	"get_sentinel_mock":    tfeTools.GetSentinelMock,

	// Terraform - Workspace Variables
	"list_workspace_variables":  tfeTools.ListWorkspaceVariables,
	"create_workspace_variable": tfeTools.CreateWorkspaceVariable,
	"update_workspace_variable": tfeTools.UpdateWorkspaceVariable,

	// Terraform - Variable Sets
	"list_variable_sets":                  tfeTools.ListVariableSets,
	"create_variable_set":                 tfeTools.CreateVariableSet,
	"create_variable_in_variable_set":     tfeTools.CreateVariableInVariableSet,
	"delete_variable_in_variable_set":     tfeTools.DeleteVariableInVariableSet,
	"attach_variable_set_to_workspaces":   tfeTools.AttachVariableSetToWorkspaces,
	"detach_variable_set_from_workspaces": tfeTools.DetachVariableSetFromWorkspaces,

	// Terraform - Tags and Policies
	"create_workspace_tags":           tfeTools.CreateWorkspaceTags,
	"read_workspace_tags":             tfeTools.ReadWorkspaceTags,
	"attach_policy_set_to_workspaces": tfeTools.AttachPolicySetToWorkspaces,
	"list_workspace_policy_sets":      tfeTools.ListWorkspacePolicySets,

	// Terraform - Stacks
	"list_stacks":       tfeTools.ListStacks,
	"get_stack_details": tfeTools.GetStackDetails,

	// Terraform - State Versions
	"list_state_versions": tfeTools.ListStateVersions,
	"get_state_version":   tfeTools.GetStateVersion,
}
