package tools

import (
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

func RegisterTools(svr *mcp.Server, logger *log.Logger, enabledToolsets []string) {
	if toolsets.IsToolEnabled("list_workspaces", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListWorkpsacesTool(), tfeTools.ListWorkspacesFunc)
	}

	if toolsets.IsToolEnabled("list_terraform_orgs", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	}

	if toolsets.IsToolEnabled("list_teams", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.ListTeamsTool(), tfeTools.ListTeamsFunc)
	}
	if toolsets.IsToolEnabled("get_team", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GetTeamTool(), tfeTools.GetTeamFunc)
	}
	if toolsets.IsToolEnabled("create_team", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.CreateTeamTool(), tfeTools.CreateTeamFunc)
	}
	if toolsets.IsToolEnabled("add_team_member", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.AddTeamMemberTool(), tfeTools.AddTeamMemberFunc)
	}
	if toolsets.IsToolEnabled("grant_team_access", enabledToolsets) {
		mcp.AddTool(svr, tfeTools.GrantTeamAccessTool(), tfeTools.GrantTeamAccessFunc)
	}
}
