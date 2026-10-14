// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestStdioServerListsRegistryTools(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	ss, cs := connectStdioServer(t)
	defer ss.Close()
	defer cs.Close()
	res, err := cs.ListTools(context.Background(), nil)

	require.NoError(t, err)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	require.True(t, names["search_provider_docs"], "registry tool should be registered")
	require.False(t, names["list_workspaces"], "TFE tool must not be registered for the registry toolset")
}

func TestStdioSessionIDFallback(t *testing.T) {
	officialclient.SetFallbackSessionID(stdioSessionID)
	t.Cleanup(func() { officialclient.SetFallbackSessionID("") })

	ss, cs := connectStdioServer(t)
	defer ss.Close()
	defer cs.Close()

	require.Empty(t, ss.ID(), "the SDK gives stdio sessions no ID")
	require.Equal(t, stdioSessionID, officialclient.SessionID(ss))
}

// connectStdioServer connects a client to a stdio-style server over the
// in-memory transport and returns both sessions
func connectStdioServer(t *testing.T) (*mcp.ServerSession, *mcp.ClientSession) {
	t.Helper()
	logger := log.New()
	logger.SetOutput(io.Discard)
	srv := newStdioServer(logger, toolsets.NewToolsetFilter([]string{toolsets.Registry}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := c.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	return ss, cs
}

func TestIsCleanShutdown(t *testing.T) {
	require.True(t, isCleanShutdown(io.EOF))
	require.True(t, isCleanShutdown(fmt.Errorf("wrapped: %w", io.EOF)))
	require.True(t, isCleanShutdown(fmt.Errorf("server is closing: %w", &jsonrpc.Error{Code: codeServerClosing})))
	require.False(t, isCleanShutdown(&jsonrpc.Error{Code: jsonrpc.CodeInternalError}))
	require.False(t, isCleanShutdown(fmt.Errorf("boom")))
}
