// Copyright IBM Corp. 2025, 2026
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
	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// stdioSessionID identifies the single session of a stdio process.
const stdioSessionID = "stdio"

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
		WithSessionLifecycle(rateLimiter, logger),
	)
}

// RunStdioServer serves the go-sdk MCP server over stdio until the client
// disconnects or the process receives a shutdown signal.
func RunStdioServer(logger *log.Logger, filter toolsets.ToolFilter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The SDK gives stdio sessions an empty ID. Use a fixed one so the cached
	// client, credentials file and per-session rate limit all work.
	officialclient.SetFallbackSessionID(stdioSessionID)

	srv := newStdioServer(logger, filter)
	_, _ = fmt.Fprintf(os.Stderr, "Terraform MCP Server (go-sdk) running on stdio\n")

	// Run blocks until the client disconnects or ctx is cancelled. A client
	// closing stdin (EOF) is a normal shutdown, same as the mark3labs server.
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil && !isCleanShutdown(err) {
		return fmt.Errorf("error running server: %w", err)
	}
	logger.Infof("shutting down server...")
	return nil
}

// codeServerClosing is the JSON-RPC code the go-sdk uses for "server is closing"
const codeServerClosing = -32004

// isCleanShutdown reports whether err from Run just means the client went
// away. A client closing stdin is EOF, or "server is closing" when a request
// was still in flight. mark3labs treats both as a normal exit.
func isCleanShutdown(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var we *jsonrpc.Error
	return errors.As(err, &we) && we.Code == codeServerClosing
}
