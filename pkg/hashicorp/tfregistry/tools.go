// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfregistry

import (
	"hcp-terraform-mcp-server/pkg/hashicorp"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

var DefaultTools = []string{"all"}

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, analytics hashicorp.Analytics, logger *log.Logger) {
	hcServer.AddTool(ProviderDetails(registryClient, analytics, logger))
	hcServer.AddTool(providerResourceDetails(registryClient, analytics, logger))
	hcServer.AddTool(ListModules(registryClient, analytics, logger))
	hcServer.AddTool(ModuleDetails(registryClient, analytics, logger))
}
