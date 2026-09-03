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

	if toolsets.IsToolEnabled("search_private_modules", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.SearchPrivateModulesTool(), tfeTools.SearchPrivateModulesFunc)
	}

	if toolsets.IsToolEnabled("search_private_providers", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.SearchPrivateProvidersTool(), tfeTools.SearchPrivateProvidersFunc)
	}

	if toolsets.IsToolEnabled("get_private_provider_details", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetPrivateProviderDetailsTool(), tfeTools.GetPrivateProviderDetailsFunc)
	}
}
