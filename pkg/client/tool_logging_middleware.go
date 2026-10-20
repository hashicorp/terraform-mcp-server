// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ToolLoggingMiddleware logs tool names and, for non-Search tools, argument keys.
// Argument values are never logged, since they can hold customer data such as
// sensitive variable values.
func ToolLoggingMiddleware(logger *log.Logger) server.ToolHandlerMiddleware {
	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if logger != nil {
				if toolsets.IsSearchTool(request.Params.Name) {
					logger.Infof("Tool call %q executed", request.Params.Name)
				} else {
					logger.Infof("Tool call %q executed with argument keys: %v", request.Params.Name, toolArgumentKeys(request.GetArguments()))
				}
			}
			return nextToolHandler(ctx, request)
		}
	}
}

// toolArgumentKeys returns the sorted top-level keys of a tool call's arguments.
// It never returns values. Missing arguments will give just an empty slice.
func toolArgumentKeys(args map[string]any) []string {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
