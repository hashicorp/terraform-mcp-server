// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/instructions"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// newStdioServer builds the go-sdk MCP server used for stdio. It is split out
// from RunStdioServer so tests can run it over an in-memory transport
func newStdioServer(logger *log.Logger, filter toolsets.ToolFilter) *mcp.Server {
	officialLogger := logging.WrapLogrus(logger).With("component", "mcp-official")
	rateLimiter := client.NewRateLimitMiddleware(client.LoadRateLimitConfigFromEnv(), logger)

	return NewServer(version.Version, instructions.Text, 0, officialLogger, filter,
		WithMiddlewares(
			middleware.RateLimit(rateLimiter),
			middleware.ToolLogging(officialLogger),
		),
		// Same session wiring as the HTTP bridge: set up clients when the
		// session starts, clean them up when the client disconnects.
		WithOnSession(func(ctx context.Context, session *mcp.ServerSession) {
			if session == nil {
				return
			}
			sessionID := session.ID()
			client.NewSessionHandler(ctx, sessionID, logger)
			go func() {
				_ = session.Wait()
				client.EndSessionHandler(context.Background(), sessionID, rateLimiter, logger)
			}()
		}),
	)
}

// RunStdioServer serves the go-sdk MCP server over stdio until the client
// disconnects or the process receives a shutdown signal.
func RunStdioServer(logger *log.Logger, filter toolsets.ToolFilter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := newStdioServer(logger, filter)
	_, _ = fmt.Fprintf(os.Stderr, "Terraform MCP Server (go-sdk) running on stdio\n")

	// Run blocks until the client disconnects or ctx is cancelled. A client
	// closing stdin (EOF) is a normal shutdown, same as the mark3labs server.
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("error running server: %w", err)
	}
	logger.Infof("shutting down server...")
	return nil
}
