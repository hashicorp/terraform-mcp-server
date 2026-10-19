// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// Resources are registered regardless of which toolsets are enabled, same as
// the mark3labs server.
func TestNewServerRegistersResources(t *testing.T) {
	svr := NewServer("test", "", 0, slog.New(slog.NewTextHandler(io.Discard, nil)),
		toolsets.NewToolsetFilter([]string{toolsets.Registry}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := svr.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer cs.Close()

	resources, err := cs.ListResources(ctx, nil)
	require.NoError(t, err)
	require.Len(t, resources.Resources, 2)

	templates, err := cs.ListResourceTemplates(ctx, nil)
	require.NoError(t, err)
	require.Len(t, templates.ResourceTemplates, 1)
}
