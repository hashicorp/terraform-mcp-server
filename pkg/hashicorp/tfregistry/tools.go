// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfregistry

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {
	hcServer.AddTool(ResolveProviderDocID(registryClient, logger))
	hcServer.AddTool(GetProviderDocs(registryClient, logger))
	hcServer.AddTool(SearchModules(registryClient, logger))
	hcServer.AddTool(ModuleDetails(registryClient, logger))
	hcServer.AddTool(SearchPolicies(registryClient, logger))
	hcServer.AddTool(PolicyDetails(registryClient, logger))
}
