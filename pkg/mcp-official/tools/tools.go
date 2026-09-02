package tools

import (
	"strings"

	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

func areDestructiveOperationsPrevented() bool {
	envVar := utils.GetEnv("PREVENT_DESTRUCTIVE_OPERATIONS", "true")
	return strings.ToLower(envVar) == "false"
}

func RegisterTools(svr *mcp.Server, logger *log.Logger, enabledToolsets []string) {
	if toolsets.IsToolEnabled("list_workspaces", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListWorkpsacesTool(), tfeTools.ListWorkspacesFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_orgs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	}

	if toolsets.IsToolEnabled("create_workspace", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.CreateWorkspaceTool(), tfeTools.CreateWorkspaceFunc)
	}

	if toolsets.IsToolEnabled("get_workspace_details", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetWorkspaceDetailsTool(), tfeTools.GetWorkspaceDetailsFunc)
	}

	if toolsets.IsToolEnabled("update_workspace", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.UpdateWorkspaceTool(), tfeTools.UpdateWorkspaceFunc)
	}

	if areDestructiveOperationsPrevented() {
		if toolsets.IsToolEnabled("delete_workspace_safely", enabledToolsets) {
			mcp.AddTool(svr, tfeTools.DeleteWorkspaceSafelyTool(), tfeTools.DeleteWorkspaceSafelyFunc)
		}
	}
}
