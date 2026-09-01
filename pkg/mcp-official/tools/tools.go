package tools

import (
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
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
		mcp.AddTool(svr, tfeTools.ListWorkpsacesTool(), tfeTools.ListWorkspacesFunc)
	},
	"list_terraform_orgs": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	},
}

func RegisterTools(svr *mcp.Server, logger *log.Logger, filter toolsets.ToolFilter) {
	for _, td := range toolsets.AllTools {
		if !filter.IsToolEnabled(td.Name) {
			continue
		}
		if register, ok := officialFactories[td.Name]; ok {
			register(svr)
		}
	}

	if toolsets.IsToolEnabled("whoami", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.WhoAmITool(), tfeTools.WhoAmIFunc)
	}
}
