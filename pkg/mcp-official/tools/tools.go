package tools

import (
	"log/slog"

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
	"read_workspace_tags": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ReadWorkspaceTagsTool(), tfeTools.ReadWorkspaceTagsFunc)
	},
	"create_workspace_tags": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.CreateWorkspaceTagsTool(), tfeTools.CreateWorkspaceTagsFunc)
	},
	"list_workspace_policy_sets": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.ListWorkspacePolicySetsTool(), tfeTools.ListWorkspacePolicySetsFunc)
	},
	"attach_policy_set_to_workspaces": func(svr *mcp.Server) {
		mcp.AddTool(svr, tfeTools.AttachPolicySetToWorkspacesTool(), tfeTools.AttachPolicySetToWorkspacesFunc)
	},
}

func RegisterTools(svr *mcp.Server, logger *slog.Logger, filter toolsets.ToolFilter) {
	for _, td := range filter.EnabledTools(nil) {
		if register, ok := officialFactories[td.Name]; ok {
			register(svr)
		}
	}
}
