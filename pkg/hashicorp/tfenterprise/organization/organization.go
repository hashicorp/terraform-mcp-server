// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package organization

import (
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func InitOrganizationTools(hcServer *server.MCPServer, tfeClient *tfe.Client, logger *log.Logger) {
	hcServer.AddTool(SearchOrganizations(tfeClient, logger))
	hcServer.AddTool(GetOrganizationDetails(tfeClient, logger))
}
