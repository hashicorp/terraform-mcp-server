// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {

	ResolveProviderDocID := ResolveProviderDocID(registryClient, logger)
	hcServer.AddTool(ResolveProviderDocID.Tool, ResolveProviderDocID.Handler)

	getProviderDocsTool := GetProviderDocs(registryClient, logger)
	hcServer.AddTool(getProviderDocsTool.Tool, getProviderDocsTool.Handler)

	getSearchModulesTool := SearchModules(registryClient, logger)
	hcServer.AddTool(getSearchModulesTool.Tool, getSearchModulesTool.Handler)

	moduleDetailsTool := ModuleDetails(registryClient, logger)
	hcServer.AddTool(moduleDetailsTool.Tool, moduleDetailsTool.Handler)

	getSearchModules := SearchModules(registryClient, logger)
	hcServer.AddTool(getSearchModules.Tool, getSearchModules.Handler)

	policyDetailsTool := PolicyDetails(registryClient, logger)
	hcServer.AddTool(policyDetailsTool.Tool, policyDetailsTool.Handler)
}
