// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/registry"
	searchTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/tfe"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// toolFactory builds a server.ServerTool (Tool + Handler) for one tool.
type toolFactory func(logger *log.Logger) server.ServerTool

// toolFactories is the single lookup both RegisterTools (tools.go, registry
// tools) and registerTFETools (dynamic_tool.go, TFE tools) dispatch through,
// keyed by toolsets.ToolDef.Name. Two tools that don't fit this plain shape
// (create_no_code_workspace, create_run) live in specialFactories below
var toolFactories = map[string]toolFactory{
	// HCP Terraform no-code search tools
	"generate_query_configuration": searchTools.GenerateQueryConfiguration,
	"provider_list_schema_list":    searchTools.ProviderListSchemaList,
	"execute_query":                searchTools.ExecuteQuery,
	"get_query_status":             searchTools.GetQueryStatus,
	"get_query_summary":            searchTools.GetQuerySummary,

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

// specialFactories covers the 2 tools that don't fit toolFactory's shape:
//   - create_no_code_workspace: also needs *server.MCPServer for elicitation
//   - create_run: picks CreateRun or CreateRunSafe based on ENABLE_TF_OPERATIONS
//
// Keeping these in a map (not a switch statement) keeps every tool's setup
// data-driven, not just most of them
var specialFactories = map[string]func(logger *log.Logger, mcpServer *server.MCPServer, tfOpsEnabled bool) server.ServerTool{
	"create_no_code_workspace": func(logger *log.Logger, mcpServer *server.MCPServer, _ bool) server.ServerTool {
		return tfeTools.CreateNoCodeWorkspace(logger, mcpServer)
	},
	"create_run": func(logger *log.Logger, _ *server.MCPServer, tfOpsEnabled bool) server.ServerTool {
		if tfOpsEnabled {
			return tfeTools.CreateRun(logger)
		}
		return tfeTools.CreateRunSafe(logger)
	},
}
