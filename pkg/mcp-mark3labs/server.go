// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/instructions"
	"github.com/hashicorp/terraform-mcp-server/pkg/resources"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func NewServer(version string, logger *log.Logger, filter toolsets.ToolFilter, opts ...server.ServerOption) (*server.MCPServer, *client.RateLimitMiddleware) {
	// filter is accepted for signature symmetry with RunHTTPServer/RunStdioServer, actual tool gating happens in registerToolsAndResources -> tools.RegisterTools.

	// Create rate limiting middleware with environment-based configuration
	rateLimitConfig := client.LoadRateLimitConfigFromEnv()
	rateLimitMiddleware := client.NewRateLimitMiddleware(rateLimitConfig, logger)

	// Add default options
	defaultOpts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithInstructions(instructions.Text),
		server.WithToolHandlerMiddleware(rateLimitMiddleware.Middleware()),
		server.WithToolHandlerMiddleware(client.ToolLoggingMiddleware(logger)),
		server.WithElicitation(),
	}
	opts = append(defaultOpts, opts...)

	// Create a new MCP server
	s := server.NewMCPServer(
		"terraform-mcp-server",
		version,
		opts...,
	)
	return s, rateLimitMiddleware
}

// registerToolsAndResources registers tools and resources with the MCP server
func registerToolsAndResources(hcServer *server.MCPServer, logger *log.Logger, filter toolsets.ToolFilter) {
	tools.RegisterTools(hcServer, logger, filter)
	resources.RegisterResources(hcServer, logger)
	resources.RegisterResourceTemplates(hcServer, logger)
}
