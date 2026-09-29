// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func ToolLoggingMiddleware(logger *log.Logger) server.ToolHandlerMiddleware {
	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if logger != nil {
				// Arguments may contain configuration, credentials or observed values.
				// Do not serialize them even before handler-side validation/redaction.
				logger.WithField("tool", request.Params.Name).Info("Tool call")
			}
			return nextToolHandler(ctx, request)
		}
	}
}
