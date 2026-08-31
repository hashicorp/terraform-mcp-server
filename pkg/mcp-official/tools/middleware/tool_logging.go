// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolLogging is the go-sdk equivalent of client.ToolLoggingMiddleware:
// it logs the tool name and arguments for every tools/call request.
func ToolLogging(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			toolName := "unknown"
			arguments := ""
			if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				toolName = params.Name
				arguments = string(params.Arguments)
			}

			result, err := next(ctx, method, req)
			// Protocol method error
			if err != nil {
				logger.ErrorContext(ctx, "tool call failed",
					"tool", toolName,
					"arguments", arguments,
					"error", err)
				return result, err
			}

			// Tool execution error
			if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
				if toolErr := toolResult.GetError(); toolErr != nil {
					logger.ErrorContext(ctx, "tool call failed",
						"tool", toolName,
						"arguments", arguments,
						"error", toolErr)
				} else {
					logger.WarnContext(ctx, "tool call returned an error result",
						"tool", toolName,
						"arguments", arguments)
				}
				return result, nil
			}

			logger.InfoContext(ctx, "tool call completed",
				"tool", toolName,
				"arguments", arguments)
			return result, nil
		}
	}
}
