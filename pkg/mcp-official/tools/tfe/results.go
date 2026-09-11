// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// textResult wraps a plain string in a CallToolResult. It is used by tools that
// return an already-serialized payload (for example a jsonapi document) or a
// human-readable status message, and therefore have no structured output type.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}
