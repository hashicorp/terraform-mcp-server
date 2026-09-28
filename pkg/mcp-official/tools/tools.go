// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"log/slog"
	"os"
	"strings"

	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/registry"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// officialFactories has one entry per tool actually ported to the official
// go-sdk so far. mcp.AddTool is generic per tool (different Arguments/Result
// types), so each entry is a closure that calls it directly rather than a
// plain map[string]func(*log.Logger) server.ServerTool like the mark3labs
// side uses. Tools that log take the server's logger from RegisterTools.

var officialFactories = map[string]func(svr *mcp.Server, logger *slog.Logger){
	// Add entries here as each tool gets ported to the official go-sdk
	"list_workspaces": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.ListWorkspacesTool(), tfeTools.ListWorkspacesFunc)
	},
	"list_terraform_orgs": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.ListTerraformOrganizationsTool(), tfeTools.ListTerraformOrganizationsFunc)
	},
	"list_terraform_projects": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.ListProjectsTool(), tfeTools.ListProjectsFunc)
	},
	"create_project": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.CreateProjectTool(), tfeTools.CreateProjectFunc)
	},
	"get_project": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.GetProjectTool(), tfeTools.GetProjectFunc)
	},
	"delete_project": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.DeleteProjectTool(), tfeTools.DeleteProjectFunc)
	},
	"whoami": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.WhoAmITool(), tfeTools.WhoAmIFunc)
	},
	"get_token_permissions": func(svr *mcp.Server, _ *slog.Logger) {
		mcp.AddTool(svr, tfeTools.GetTokenPermissionsTool(), tfeTools.GetTokenPermissionsFunc)
	},
	"search_providers": func(svr *mcp.Server, logger *slog.Logger) {
		mcp.AddTool(svr, registryTools.SearchProvidersTool(), registryTools.SearchProvidersFunc(logger))
	},
	"get_provider_details": func(svr *mcp.Server, logger *slog.Logger) {
		mcp.AddTool(svr, registryTools.GetProviderDetailsTool(), registryTools.GetProviderDetailsFunc(logger))
	},
	"get_latest_provider_version": func(svr *mcp.Server, logger *slog.Logger) {
		mcp.AddTool(svr, registryTools.GetLatestProviderVersionTool(), registryTools.GetLatestProviderVersionFunc(logger))
	},
	"get_provider_capabilities": func(svr *mcp.Server, logger *slog.Logger) {
		mcp.AddTool(svr, registryTools.GetProviderCapabilitiesTool(), registryTools.GetProviderCapabilitiesFunc(logger))
	},
}

func RegisterTools(svr *mcp.Server, logger *slog.Logger, filter toolsets.ToolFilter) {
	tfOpsEnabled := strings.EqualFold(os.Getenv("ENABLE_TF_OPERATIONS"), "true")

	for _, td := range filter.EnabledTools(nil) {
		if td.RequiresTFOps && !tfOpsEnabled {
			continue
		}
		if register, ok := officialFactories[td.Name]; ok {
			register(svr, logger)
		}
	}
}
