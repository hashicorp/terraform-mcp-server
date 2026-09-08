// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"time"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// Metrics is the go-sdk equivalent of main.go's attachMetricsHooks. Unlike
// the mark3labs hooks, it doesn't need a sessionClientInfo cache populated by
// a separate AddAfterInitialize hook — ServerSession.InitializeParams()
// already gives us the client info directly.
func Metrics(metricsConfig tfeclient.MetricsConfig, logger *log.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" || !metricsConfig.Enabled {
				return next(ctx, method, req)
			}
			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok {
				return next(ctx, method, req)
			}

			if session, ok := req.GetSession().(*mcp.ServerSession); ok && session != nil {
				if ip := session.InitializeParams(); ip != nil && ip.ClientInfo != nil {
					tfeclient.RecordClientType(ctx, tfeclient.ClientInfo{
						Name:        ip.ClientInfo.Name,
						Version:     ip.ClientInfo.Version,
						Title:       ip.ClientInfo.Title,
						Description: ip.ClientInfo.Description,
					}, metricsConfig, logger)
				}
			}

			start := time.Now()
			result, err := next(ctx, method, req)

			toolErr := err != nil
			if res, ok := result.(*mcp.CallToolResult); ok && res != nil && res.IsError {
				toolErr = true
			}
			tfeclient.RecordToolCallByName(ctx, start, toolErr, params.Name, metricsConfig, logger)
			return result, err
		}
	}
}
