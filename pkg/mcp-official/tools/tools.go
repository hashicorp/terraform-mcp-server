package tools

import (
	"strings"

	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

func areDestructiveOperationsAllowed() bool {
	envVar := utils.GetEnv("PREVENT_DESTRUCTIVE_OPERATIONS", "true")
	return strings.ToLower(envVar) == "false"
}

func RegisterTools(svr *mcp.Server, logger *log.Logger, enabledToolsets []string) {
	if toolsets.IsToolEnabled("list_workspaces", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListWorkspacesTool(), tfeTools.ListWorkspacesFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_orgs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_projects", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListProjectsTool(), tfeTools.ListProjectsFunc)
	}

	if toolsets.IsToolEnabled("create_project", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.CreateProjectTool(), tfeTools.CreateProjectFunc)
	}

	if toolsets.IsToolEnabled("get_project", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetProjectTool(), tfeTools.GetProjectFunc)
	}

	if areDestructiveOperationsAllowed() {
		if toolsets.IsToolEnabled("delete_project", enabledToolsets) {
			mcp.AddTool(svr, tfeTools.DeleteProjectTool(), tfeTools.DeleteProjectFunc)
		}
	}
}
