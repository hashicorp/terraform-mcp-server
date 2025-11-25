package server

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/resources"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

//go:embed instructions.md
var instructions string

func NewServer(version string, logger *log.Logger, opts ...server.ServerOption) *server.MCPServer {
	// Create rate limiting middleware with environment-based configuration
	rateLimitConfig := client.LoadRateLimitConfigFromEnv()
	rateLimitMiddleware := client.NewRateLimitMiddleware(rateLimitConfig, logger)

	// Add default options
	defaultOpts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithInstructions(instructions),
		server.WithToolHandlerMiddleware(rateLimitMiddleware.Middleware()),
		server.WithElicitation(),
	}
	opts = append(defaultOpts, opts...)

	// Create hooks for session management
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		client.NewSessionHandler(ctx, session, logger)
	})
	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		client.EndSessionHandler(ctx, session, logger)
	})

	// Add hooks to options
	opts = append(opts, server.WithHooks(hooks))

	// Create a new MCP server
	s := server.NewMCPServer(
		"terraform-mcp-server",
		version,
		opts...,
	)

	registerToolsAndResources(s, logger)
	return s
}

// registerToolsAndResources registers tools and resources with the MCP server
func registerToolsAndResources(hcServer *server.MCPServer, logger *log.Logger) {
	tools.RegisterTools(hcServer, logger)
	resources.RegisterResources(hcServer, logger)
	resources.RegisterResourceTemplates(hcServer, logger)
}
