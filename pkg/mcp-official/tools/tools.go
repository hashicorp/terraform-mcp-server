package tools

import (
	"log/slog"

	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/registry"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// officialFactories has one entry per tool actually ported to the official
// go-sdk so far. mcp.AddTool is generic per tool (different Arguments/Result
// types), so each entry is a closure that calls it directly rather than a
// plain map[string]func(*log.Logger) server.ServerTool like the mark3labs
// side uses

var officialFactories = map[string]func(svr *mcp.Server){
	// Add entries here as each tool gets ported to the official go-sdk
	"list_workspaces": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListWorkspacesTool(), tfeTools.ListWorkspacesFunc)
	},
	"list_terraform_orgs": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	},
	"search_modules": func(svr *mcp.Server) {
		mcp.AddTool(svr, registryTools.SearchModulesTool(), registryTools.SearchModulesFunc)
	},
	"get_latest_module_version": func(svr *mcp.Server) {
		mcp.AddTool(svr, registryTools.GetLatestModuleVersionTool(), registryTools.GetLatestModuleVersionFunc)
	},
	"get_module_details": func(svr *mcp.Server) {
		mcp.AddTool(svr, registryTools.GetModuleDetailsTool(), registryTools.GetModuleDetailsFunc)
	},
}

func RegisterTools(svr *mcp.Server, logger *slog.Logger, filter toolsets.ToolFilter) {
	for _, td := range filter.EnabledTools(nil) {
		if register, ok := officialFactories[td.Name]; ok {
			register(svr)
		}
	}
}
