// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	registryTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/registry"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func RegisterTools(hcServer *server.MCPServer, logger *log.Logger) {
	// Register the dynamic tool
	registerDynamicTools(hcServer, logger)

	// Provider tools (always available)
	getResolveProviderDocIDTool := registryTools.ResolveProviderDocID(logger)
	hcServer.AddTool(getResolveProviderDocIDTool.Tool, getResolveProviderDocIDTool.Handler)

	getProviderDocsTool := registryTools.GetProviderDocs(logger)
	hcServer.AddTool(getProviderDocsTool.Tool, getProviderDocsTool.Handler)

	getLatestProviderVersionTool := registryTools.GetLatestProviderVersion(logger)
	hcServer.AddTool(getLatestProviderVersionTool.Tool, getLatestProviderVersionTool.Handler)

	// Module tools
	getSearchModulesTool := registryTools.SearchModules(logger)
	hcServer.AddTool(getSearchModulesTool.Tool, getSearchModulesTool.Handler)

	getModuleDetailsTool := registryTools.ModuleDetails(logger)
	hcServer.AddTool(getModuleDetailsTool.Tool, getModuleDetailsTool.Handler)

	getLatestModuleVersionTool := registryTools.GetLatestModuleVersion(logger)
	hcServer.AddTool(getLatestModuleVersionTool.Tool, getLatestModuleVersionTool.Handler)

	// Policy tools
	getSearchPoliciesTool := registryTools.SearchPolicies(logger)
	hcServer.AddTool(getSearchPoliciesTool.Tool, getSearchPoliciesTool.Handler)

	getPolicyDetailsTool := registryTools.PolicyDetails(logger)
	hcServer.AddTool(getPolicyDetailsTool.Tool, getPolicyDetailsTool.Handler)
}
