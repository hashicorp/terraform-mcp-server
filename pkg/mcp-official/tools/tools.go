// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"log/slog"
	"strings"

	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// officialFactories has one entry per tool actually ported to the official
// go-sdk so far. mcp.AddTool is generic per tool (different Arguments/Result
// types), so each entry is a closure that calls it directly rather than a
// plain map[string]func(*log.Logger) server.ServerTool like the mark3labs
// side uses

var officialFactories = map[string]func(svr *mcp.Server){
	// Add entries here as each tool gets ported to the official go-sdk
	// Tools not yet migrated simply have no entry and are silently skipped
	// below — no other code needs to change.
	"list_workspaces": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListWorkspacesTool(), tfeTools.ListWorkspacesFunc)
	},
	"list_terraform_orgs": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	},
	"list_terraform_projects": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListProjectsTool(), tfeTools.ListProjectsFunc)
	},
	"create_project": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.CreateProjectTool(), tfeTools.CreateProjectFunc)
	},
	"get_project": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.GetProjectTool(), tfeTools.GetProjectFunc)
	},
	"delete_project": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.DeleteProjectTool(), tfeTools.DeleteProjectFunc)
	},
}

// isTerraformOperationsEnabled checks if ENABLE_TF_OPERATIONS is set to true
func isTerraformOperationsEnabled() bool {
	envVar := utils.GetEnv("ENABLE_TF_OPERATIONS", "false")
	return strings.ToLower(envVar) == "true"
}

func RegisterTools(svr *mcp.Server, logger *slog.Logger, filter toolsets.ToolFilter) {

	tfOpsEnabled := isTerraformOperationsEnabled()

	for _, td := range toolsets.AllTools {
		if !filter.IsToolEnabled(td.Name) {
			continue
		}

		if td.RequiresTFOps && !tfOpsEnabled {
			continue
		}

		if register, ok := officialFactories[td.Name]; ok {
			register(svr)
		}
	}
}
