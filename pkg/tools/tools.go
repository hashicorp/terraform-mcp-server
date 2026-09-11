// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
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

}
