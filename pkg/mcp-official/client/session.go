// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import "github.com/modelcontextprotocol/go-sdk/mcp"

// SessionIDFromRequest returns the session ID attached to a request, or ""
// when the request has no session (stateless mode)
func SessionIDFromRequest(request *mcp.CallToolRequest) string {
	if request == nil || request.Session == nil {
		return ""
	}
	return request.Session.ID()
}
