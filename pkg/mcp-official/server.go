package mcpofficial

import (
	"time"

	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServer(heartbeatInterval time.Duration, logger *log.Logger, enabledToolsets []string) *mcp.Server {
	var svrOptions *mcp.ServerOptions
	if heartbeatInterval > 0 {
		log.Infof("HTTP heartbeat enabled with interval: %v", heartbeatInterval)
		svrOptions = &mcp.ServerOptions{
			KeepAlive: heartbeatInterval,
		}
	}
	svr := mcp.NewServer(
		&mcp.Implementation{
			Name:    "terraform-mcp-official",
			Version: version.Version,
		},
		svrOptions,
	)
	logger.Info("Official mcp go-sdk server instance started at 127.0.0.1:8080/mcp/official..")
	RegisterTools(svr, logger, enabledToolsets)
	return svr
}
