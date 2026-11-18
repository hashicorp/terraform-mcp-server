// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/instructions"
	mcpofficial "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// getOfficialStreamableServer builds the go-sdk StreamableHTTP handler that
// streamableHTTPServerInit mounts at /mcp/official when
// TF_X_OFFICIAL_SDK_ENABLED is set. It bridges rate limiting, session
// cleanup, metrics, and security wrapping to the go-sdk server so it behaves
// the same as the mark3labs server it sits alongside.
func getOfficialStreamableServer(ctx context.Context, heartbeatInterval time.Duration, isStateless bool, corsConfig client.CORSConfig, logger *log.Logger, officialLogger *slog.Logger, organizationAllowlist []string, filter toolsets.ToolFilter, rateLimiter *client.RateLimitMiddleware, metricsConfig client.MetricsConfig) http.Handler {
	officialLogger.Info("Creating a go-sdk StreamableHTTP server...")

	// Metrics is added first so it wraps every other middleware, measuring
	// the full round trip and always recording the result — the go-sdk
	// equivalent of AddBeforeCallTool/AddAfterCallTool wrapping the whole
	// mark3labs middleware chain in attachMetricsHooks
	middlewares := []mcp.Middleware{
		middleware.Metrics(metricsConfig, logger),
		middleware.RateLimit(rateLimiter),
		middleware.ToolLogging(officialLogger),
	}
	if len(organizationAllowlist) > 0 {
		middlewares = append(middlewares, middleware.OrganizationAllowlist(organizationAllowlist, officialLogger))
	}
	serverOpts := []mcpofficial.Option{
		mcpofficial.WithMiddlewares(middlewares...),
		// Creates each session's clients on start and cleans them up when the
		// client disconnects (shared with the stdio runner)
		mcpofficial.WithSessionLifecycle(rateLimiter, logger),
	}
	hcServer := mcpofficial.NewServer(version.Version, instructions.Text, heartbeatInterval, officialLogger, filter, serverOpts...)

	opts := &mcp.StreamableHTTPOptions{
		Stateless:                  isStateless,
		Logger:                     officialLogger,
		CrossOriginProtection:      nil, // disables the SDK's built-in cross-origin protection entirely. CORS already enforced by client.NewSecurityHandler below.
		DisableLocalhostProtection: shouldDisableLocalhostProtection(),
	}

	// Create the base MCP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return hcServer
	}, opts)

	// RequireTerraformTokenMiddleware is a no-op unless MCP_REQUIRE_TOKEN is true.
	streamableServer := client.RequireTerraformTokenMiddleware(logger)(mcpHandler)
	streamableServer = client.OrganizationAllowlistMiddleware(organizationAllowlist, logger)(streamableServer)
	streamableServer = client.TerraformContextMiddleware(logger)(streamableServer)
	streamableServer = client.NewSecurityHandler(streamableServer, corsConfig.AllowedOrigins, corsConfig.Mode, logger)
	return streamableServer
}
