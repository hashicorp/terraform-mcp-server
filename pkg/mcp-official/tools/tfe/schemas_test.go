// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// listedTool registers one tool on a real server and returns it as a client sees it
// through tools/list. Schema problems surface nowhere else: the SDK derives the output
// schema from the handler at registration, and a strict client silently drops a tool whose
// schema root is not an object, so neither a handler call nor an HCPT run would notice.
func listedTool[In, Out any](t *testing.T, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) *mcp.Tool {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	mcp.AddTool(server, tool, handler)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { serverSession.Close() })

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { clientSession.Close() })

	listed, err := clientSession.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1)

	return listed.Tools[0]
}

// objectSchema asserts that a listed schema is an object at the root, which is the shape
// the published MCP Tool definition requires, and returns its decoded properties.
func objectSchema(t *testing.T, schema any, label string) map[string]any {
	t.Helper()

	decoded, ok := schema.(map[string]any)
	require.True(t, ok, "%s should decode to a JSON object", label)
	require.Equal(t, "object", decoded["type"], "%s must be object-rooted or strict clients drop the tool", label)

	properties, ok := decoded["properties"].(map[string]any)
	require.True(t, ok, "%s should declare properties", label)
	return properties
}
