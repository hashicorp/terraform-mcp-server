// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolLoggingMiddleware is the go-sdk equivalent of client.ToolLoggingMiddleware:
// it logs the tool name and arguments for every tools/call request.
func ToolLoggingMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && logger != nil {
				logger.InfoContext(ctx, "tool call executed",
					"tool", params.Name, "arguments", string(params.Arguments))
			}

			return next(ctx, method, req)
		}
	}
}
