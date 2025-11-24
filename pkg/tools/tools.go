// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/registry"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func RegisterTools(hcServer *server.MCPServer, logger *log.Logger, enabledToolsets []string) {
	// Register the dynamic tools (TFE tools that require authentication)
	registerDynamicTools(hcServer, logger, enabledToolsets)

	// Registry toolset - Provider tools
	if toolsets.IsToolEnabled("search_providers", enabledToolsets) {
		tool := registryTools.ResolveProviderDocID(logger)
		hcServer.AddTool(tool.Tool, tool.Handler)
	}

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
}
