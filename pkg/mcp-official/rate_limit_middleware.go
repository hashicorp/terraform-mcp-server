// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RateLimitMiddleware is the go-sdk equivalent of client.RateLimitMiddleware.Middleware():
// it enforces the same global and per-session limits via the shared rl.
func RateLimitMiddleware(rl *tfeclient.RateLimitMiddleware) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok {
				return next(ctx, method, req)
			}

			sessionID := ""
			if session, ok := req.GetSession().(*mcp.ServerSession); ok && session != nil {
				sessionID = session.ID()
			}

			if err := rl.Allow(sessionID, params.Name); err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				}, nil
			}

			return next(ctx, method, req)
		}
	}
}
