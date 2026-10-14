// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fallbackSessionID is used when the SDK gives a session with no ID. Only stdio
// sets it (one process, one client). HTTP stateless mode leaves it empty so
// every request still gets a fresh client.
var fallbackSessionID atomic.Value

// SetFallbackSessionID sets the ID used for sessions with no ID
func SetFallbackSessionID(id string) { fallbackSessionID.Store(id) }

// SessionID returns the session's ID, or the fallback ID when it has none
func SessionID(session *mcp.ServerSession) string {
	if session != nil {
		if id := session.ID(); id != "" {
			return id
		}
	}
	id, _ := fallbackSessionID.Load().(string)
	return id
}

// SessionIDFromRequest returns the session ID attached to a request, or ""
// when the request has no session (stateless mode)
func SessionIDFromRequest(request *mcp.CallToolRequest) string {
	if request == nil || request.Session == nil {
		return ""
	}
	return SessionID(request.Session)
}
