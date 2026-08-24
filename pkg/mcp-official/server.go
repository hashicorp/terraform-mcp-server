package mcpofficial

import (
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools"
	log "github.com/sirupsen/logrus"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverConfig holds all server config so future phases (middleware, hooks)
// don't require changing NewServer's signature again.
type serverConfig struct {
	mcpOpts     mcp.ServerOptions
	middlewares []mcp.Middleware           // consumed in Phase 2
	onSession   []func(*mcp.ServerSession) // consumed in Phase 3
}

type Option func(*serverConfig)

func NewServer(version, instructions string, heartbeatInterval time.Duration, logger *log.Logger, enabledToolsets []string, opts ...Option) *mcp.Server {
	cfg := &serverConfig{mcpOpts: mcp.ServerOptions{Instructions: instructions}}
	if heartbeatInterval > 0 {
		logger.Infof("HTTP heartbeat enabled with interval: %v", heartbeatInterval)
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
