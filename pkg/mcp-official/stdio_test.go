// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestStdioServerListsRegistryTools(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	srv := newStdioServer(logger, toolsets.NewToolsetFilter([]string{toolsets.Registry}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverT, clientT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	clientSession, err := c.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	res, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	require.True(t, names["search_providers"], "registry tool should be registered")
	require.False(t, names["list_workspaces"], "TFE tool must not be registered for the registry toolset")
}

func TestStdioServerSessionEndsOnClientClose(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	srv := newStdioServer(logger, toolsets.NewToolsetFilter([]string{toolsets.Registry}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverT, clientT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	require.NoError(t, err)

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	clientSession, err := c.Connect(ctx, clientT, nil)
	require.NoError(t, err)

	require.NoError(t, clientSession.Close())

	// Wait must return once the client is gone, proving the cleanup goroutine can fire
	done := make(chan struct{})
	go func() { _ = serverSession.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server session did not end after client closed")
	}
}
