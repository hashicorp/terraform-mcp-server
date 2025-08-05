// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {

	// Provider tools
	getResolveProviderDocIDTool := ResolveProviderDocID(registryClient, logger)
	hcServer.AddTool(getResolveProviderDocIDTool.Tool, getResolveProviderDocIDTool.Handler)

	getProviderDocsTool := GetProviderDocs(registryClient, logger)
	hcServer.AddTool(getProviderDocsTool.Tool, getProviderDocsTool.Handler)

	getLatestProviderVersionTool := GetLatestProviderVersion(registryClient, logger)
	hcServer.AddTool(getLatestProviderVersionTool.Tool, getLatestProviderVersionTool.Handler)

	// Module tools
	getSearchModulesTool := SearchModules(registryClient, logger)
	hcServer.AddTool(getSearchModulesTool.Tool, getSearchModulesTool.Handler)

	getModuleDetailsTool := ModuleDetails(registryClient, logger)
	hcServer.AddTool(getModuleDetailsTool.Tool, getModuleDetailsTool.Handler)

	getLatestModuleVersionTool := GetLatestModuleVersion(registryClient, logger)
	hcServer.AddTool(getLatestModuleVersionTool.Tool, getLatestModuleVersionTool.Handler)

	// Policy tools
	getSearchPoliciesTool := SearchPolicies(registryClient, logger)
	hcServer.AddTool(getSearchPoliciesTool.Tool, getSearchPoliciesTool.Handler)

	getPolicyDetailsTool := PolicyDetails(registryClient, logger)
	hcServer.AddTool(getPolicyDetailsTool.Tool, getPolicyDetailsTool.Handler)
}
