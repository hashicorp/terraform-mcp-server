// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"

	importworkflow "github.com/hashicorp/terraform-mcp-server/pkg/tools/search/import"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ImportQueryResults registers the Search-to-import workflow as one MCP tool.
// The second argument remains source-compatible with existing registrations.
func ImportQueryResults(logger *log.Logger, _ *server.MCPServer) server.ServerTool {
	return server.ServerTool{
		Tool: importworkflow.ToolDefinition(),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return importworkflow.Handle(ctx, request, logger)
		},
	}
}
