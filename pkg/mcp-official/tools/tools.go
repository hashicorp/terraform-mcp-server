package tools

import (
	"strings"

	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// isTerraformOperationsEnabled checks if ENABLE_TF_OPERATIONS is set to true
func isTerraformOperationsEnabled() bool {
	return strings.EqualFold(utils.GetEnv("ENABLE_TF_OPERATIONS", "false"), "true")
}

func RegisterTools(svr *mcp.Server, logger *log.Logger, enabledToolsets []string) {
	if toolsets.IsToolEnabled("list_workspaces", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListWorkpsacesTool(), tfeTools.ListWorkspacesFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_orgs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	}

	if toolsets.IsToolEnabled("list_runs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListRunsTool(), tfeTools.ListRunsFunc)
	}

	if toolsets.IsToolEnabled("create_run", enabledToolsets) {
		if isTerraformOperationsEnabled() {
			mcp.AddTool(svr, tfeTools.CreateRunTool(), tfeTools.CreateRunFunc)
		} else {
			mcp.AddTool(svr, tfeTools.CreateRunSafeTool(), tfeTools.CreateRunSafeFunc)
		}
	}

	if isTerraformOperationsEnabled() && toolsets.IsToolEnabled("action_run", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ActionRunTool(), tfeTools.ActionRunFunc)
	}

	if toolsets.IsToolEnabled("get_run_details", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetRunDetailsTool(), tfeTools.GetRunDetailsFunc)
	}

	if toolsets.IsToolEnabled("get_run_comments", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetRunCommentsTool(), tfeTools.GetRunCommentsFunc)
	}
}
