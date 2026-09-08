// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// serverConfig holds server settings, set via Options like WithMiddlewares
// and WithOnSession, so NewServer's signature stays stable as we add more.
type serverConfig struct {
	mcpOpts     mcp.ServerOptions
	middlewares []mcp.Middleware
	onSession   []func(context.Context, *mcp.ServerSession)
}

type Option func(*serverConfig)

// WithOnSession registers fn to run once per session, right after the client
// finishes MCP initialization ("notifications/initialized"). fn is called
// synchronously, so any long-running cleanup (e.g. waiting for the session
// to end) should be started in its own goroutine.
func WithOnSession(fn func(context.Context, *mcp.ServerSession)) Option {
	return func(cfg *serverConfig) {
		cfg.onSession = append(cfg.onSession, fn)
	}
}

// WithMiddlewares attaches receiving middleware to the server. Middleware
// runs on every incoming MCP request (tool calls, list calls, etc.) — each
// middleware can inspect/reject a request before it reaches its handler, or
// pass it through with next(ctx, method, req).
func WithMiddlewares(mw ...mcp.Middleware) Option {
	return func(cfg *serverConfig) {
		cfg.middlewares = append(cfg.middlewares, mw...)
	}
}

func NewServer(version, instructions string, heartbeatInterval time.Duration, logger *log.Logger, enabledToolsets []string, opts ...Option) *mcp.Server {
	cfg := &serverConfig{mcpOpts: mcp.ServerOptions{Instructions: instructions}}
	if heartbeatInterval > 0 {
		logger.Infof("HTTP heartbeat enabled with interval: %v", heartbeatInterval)
		cfg.mcpOpts.KeepAlive = heartbeatInterval
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if len(cfg.onSession) > 0 {
		cfg.mcpOpts.InitializedHandler = func(ctx context.Context, req *mcp.InitializedRequest) {
			session, _ := req.GetSession().(*mcp.ServerSession)
			for _, fn := range cfg.onSession {
				fn(ctx, session)
			}
		}
	}

	svr := mcp.NewServer(&mcp.Implementation{Name: "terraform-mcp-official", Version: version}, &cfg.mcpOpts)

	// TODO - Must attach after construction - AddReceivingMiddleware isn't a ServerOptions field.
	if len(cfg.middlewares) > 0 {
		svr.AddReceivingMiddleware(cfg.middlewares...)
	}

	tools.RegisterTools(svr, logger, enabledToolsets)
	return svr
}
