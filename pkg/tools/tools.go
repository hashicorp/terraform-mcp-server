// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/registry"
	searchTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func RegisterTools(hcServer *server.MCPServer, logger *log.Logger, filter toolsets.ToolFilter) {

	// Register the dynamic tools (TFE tools that require authentication)
	registerDynamicTools(hcServer, logger, filter)

	// Every tool that does NOT require a TFE session (i.e. the public
	// Registry toolset). TFE-gated tools are handled by
	// registerDynamicTools/registerTFETools instead, since they can only be
	// registered once a session actually has a valid TFE client.
	for _, td := range toolsets.AllTools {
		if td.RequiresTFE || !filter.IsToolEnabled(td.Name) {
			continue
		}
		factory, ok := toolFactories[td.Name]
		if !ok {
			logger.Warnf("no tool factory registered for %q; skipping", td.Name)
			continue
		}
		tool := factory(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

<<<<<<< HEAD
=======
	if toolsets.IsToolEnabled("get_provider_details", enabledToolsets) {
		tool := registryTools.GetProviderDocs(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_latest_provider_version", enabledToolsets) {
		tool := registryTools.GetLatestProviderVersion(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_provider_capabilities", enabledToolsets) {
		tool := registryTools.GetProviderCapabilities(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	// Registry toolset - Module tools
	if toolsets.IsToolEnabled("search_modules", enabledToolsets) {
		tool := registryTools.SearchModules(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_module_details", enabledToolsets) {
		tool := registryTools.ModuleDetails(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_latest_module_version", enabledToolsets) {
		tool := registryTools.GetLatestModuleVersion(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	// Registry toolset - Policy tools
	if toolsets.IsToolEnabled("search_policies", enabledToolsets) {
		tool := registryTools.SearchPolicies(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_policy_details", enabledToolsets) {
		tool := registryTools.PolicyDetails(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

	// Search toolset - No-Code Query Configuration
	if toolsets.IsToolEnabled("generate_query_configuration", enabledToolsets) {
		tool := searchTools.GenerateQueryConfiguration(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}
>>>>>>> 9312550 (Adds MCP tools list_resources_schema and generate_query_config for Terraform Search)
}
