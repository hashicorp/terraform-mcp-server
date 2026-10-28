// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// argumentErrorPrefixes are how the SDK starts its errors when tool arguments don't
// parse or don't match the input schema. Those errors can include the bad value, so
// we only log the prefix for them.
var argumentErrorPrefixes = []string{
	`validating "arguments"`,
	"unmarshaling arguments",
}

// ToolLogging is the go-sdk equivalent of client.ToolLoggingMiddleware:
// it logs the tool name and argument keys for every tools/call request.
// Argument values are never logged, since they can hold customer data such as
// sensitive variable values.
func ToolLogging(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" || logger == nil {
				return next(ctx, method, req)
			}

			params := req.GetParams().(*mcp.CallToolParamsRaw)
			toolName := params.Name
			keys := argumentKeys(params.Arguments)

			result, err := next(ctx, method, req)
			// Protocol-level error
			if err != nil {
				logger.ErrorContext(ctx, "tool call failed",
					"tool", toolName,
					"argument_keys", keys,
					"error", loggableError(err))
				return result, err
			}

			// Tool-level error
			if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
				logger.ErrorContext(ctx, "tool call failed",
					"tool", toolName,
					"argument_keys", keys,
					"error", loggableError(toolResult.GetError()))
				return result, nil
			}

			logger.InfoContext(ctx, "tool call completed",
				"tool", toolName,
				"argument_keys", keys)
			return result, nil
		}
	}
}

// argumentKeys returns the sorted top-level keys of a tool call's arguments.
// It never returns values. Missing or malformed arguments give an empty slice.
func argumentKeys(raw json.RawMessage) []string {
	keys := make([]string, 0)
	if len(raw) == 0 {
		return keys
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return keys
	}
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// loggableError returns the error message to log. If it's one of the SDK's argument
// errors we drop everything after the prefix, since the rest can include the value
// that failed. Errors from our own handlers are logged as is.
func loggableError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for _, prefix := range argumentErrorPrefixes {
		if strings.Contains(msg, prefix) {
			return prefix + ": details redacted"
		}
	}
	return msg
}
