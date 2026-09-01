package tools

import (
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
)

func RegisterTools(svr *mcp.Server, logger *log.Logger, enabledToolsets []string) {
	if toolsets.IsToolEnabled("list_workspaces", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListWorkpsacesTool(), tfeTools.ListWorkspacesFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_orgs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	}

	if toolsets.IsToolEnabled("whoami", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.WhoAmITool(), tfeTools.WhoAmIFunc)
	}

	if toolsets.IsToolEnabled("get_token_permissions", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetTokenPermissionsTool(), tfeTools.GetTokenPermissionsFunc)
	}
}
