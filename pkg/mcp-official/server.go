// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"log/slog"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverConfig holds all server config so future phases (middleware, hooks)
// don't require changing NewServer's signature again.
type serverConfig struct {
	mcpOpts     mcp.ServerOptions
	middlewares []mcp.Middleware
	onSession   []func(*mcp.ServerSession) // consumed in Phase 3
}

type Option func(*serverConfig)

// WithMiddlewares attaches receiving middleware to the server. Middleware
// runs on every incoming MCP request (tool calls, list calls, etc.) — each
// middleware can inspect/reject a request before it reaches its handler, or
// pass it through with next(ctx, method, req).
func WithMiddlewares(mw ...mcp.Middleware) Option {
	return func(cfg *serverConfig) {
		cfg.middlewares = append(cfg.middlewares, mw...)
	}
}

func NewServer(version, instructions string, heartbeatInterval time.Duration, logger *slog.Logger, enabledToolsets []string, opts ...Option) *mcp.Server {
	cfg := &serverConfig{mcpOpts: mcp.ServerOptions{Instructions: instructions, Logger: logger}}
	if heartbeatInterval > 0 {
		logger.Info("HTTP heartbeat enabled", "interval", heartbeatInterval)
		cfg.mcpOpts.KeepAlive = heartbeatInterval
	}
	for _, opt := range opts {
		opt(cfg)
	}

	svr := mcp.NewServer(&mcp.Implementation{Name: "terraform-mcp-official", Version: version}, &cfg.mcpOpts)

	// TODO - Must attach after construction - AddReceivingMiddleware isn't a ServerOptions field.
	if len(cfg.middlewares) > 0 {
		svr.AddReceivingMiddleware(cfg.middlewares...)
	}

	tools.RegisterTools(svr, logger, enabledToolsets)
	return svr
}
