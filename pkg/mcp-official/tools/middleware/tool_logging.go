// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"log/slog"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolLogging is the go-sdk equivalent of client.ToolLoggingMiddleware:
// it logs the tool name and arguments for every tools/call request.
func ToolLogging(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" || logger == nil {
				return next(ctx, method, req)
			}

			params := req.GetParams().(*mcp.CallToolParamsRaw)
			toolName := params.Name
			arguments := params.Arguments
			if toolset, ok := toolsets.GetToolsetForTool(toolName); ok && toolset == toolsets.Search {
				// Neither untrusted arguments nor backend errors belong in Search logs.
				result, err := next(ctx, method, req)
				if err != nil {
					logger.ErrorContext(ctx, "Search tool call failed", "tool", toolName)
				} else if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
					logger.ErrorContext(ctx, "Search tool call failed", "tool", toolName)
				} else {
					logger.InfoContext(ctx, "Search tool call completed", "tool", toolName)
				}
				return result, err
			}

			result, err := next(ctx, method, req)
			// Protocol-level error
			if err != nil {
				logger.ErrorContext(ctx, "tool call failed",
					"tool", toolName,
					"arguments", arguments,
					"error", err)
				return result, err
			}

			// Tool-level error
			if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
				logger.ErrorContext(ctx, "tool call failed",
					"tool", toolName,
					"arguments", arguments,
					"error", toolResult.GetError())
				return result, nil
			}

			logger.InfoContext(ctx, "tool call completed",
				"tool", toolName,
				"arguments", arguments)
			return result, nil
		}
	}
}
