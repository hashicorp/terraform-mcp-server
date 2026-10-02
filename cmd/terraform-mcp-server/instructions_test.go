// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	mcpofficial "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerInstructionsPreserveExistingWorkflowsAndGuideSearchImport(t *testing.T) {
	for _, section := range []string{
		"## Tool Usage Guidelines", "### Registry Tools", "### Private Registry Tools",
		"### Workspace Management", "### Run Execution", "### Variable Management",
		"**Code Generation**:", "**Run Management**:", "**Variable Configuration**:",
		"## Error Handling", "## Security Notes",
	} {
		assert.Contains(t, instructions, section)
	}
	for _, guidance := range []string{
		"preserve its constraints and lock selections", "Do not substitute a public provider or module for a private source",
		"Local validation does not replace the destination workspace's plan",
		"**Search-to-Import (when Search tools are enabled)**:",
		"`get_query_summary`", "Explicitly", "select up to 100", "`prepare_import` once", "`identity_support`",
		"Search provider version, observations, and generated HCL are source",
		"preserves the complete tree and provider lock", "MCP does not download, edit, or upload archive bytes",
		"`get_import_configuration_download`", "`create_import_cv`", "`create_import_run`", "`verify_import_plan`",
		"explicit confirmation for each speculative configuration-version", "uncertain create must be reconciled",
		"full finished plan", "refresh drift", "A plan does not", "separate review and approval",
	} {
		assert.Contains(t, instructions, guidance)
	}
	assert.NotContains(t, instructions, "Query registries for latest provider/module versions")
	assert.NotContains(t, instructions, "public as fallback")
}

func TestServerInitializationDeliversSearchImportGuidance(t *testing.T) {
	for _, transport := range []string{"in_process", "streamable_http"} {
		t.Run(transport, func(t *testing.T) {
			s, _ := NewServer("test", metricsTestLogger(), toolsets.NewToolsetFilter([]string{toolsets.Registry}))
			var c *mcpclient.Client
			var err error
			if transport == "in_process" {
				c, err = mcpclient.NewInProcessClient(s)
			} else {
				h := mcpserver.NewTestStreamableHTTPServer(s)
				t.Cleanup(h.Close)
				c, err = mcpclient.NewStreamableHttpClient(h.URL)
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, c.Close()) })
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, c.Start(ctx))
			result, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "instructions-test", Version: "1"}}})
			require.NoError(t, err)
			assert.Equal(t, instructions, result.Instructions)
			assert.Contains(t, result.Instructions, "**Search-to-Import (when Search tools are enabled)**")
		})
	}
}

func TestOfficialServerInitializationUsesSameInstructions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := mcpofficial.NewServer("test", instructions, 0, slog.New(slog.NewTextHandler(io.Discard, nil)), toolsets.NewToolsetFilter([]string{toolsets.Registry}))
	clientTransport, serverTransport := officialmcp.NewInMemoryTransports()
	serverSession, err := s.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	c := officialmcp.NewClient(&officialmcp.Implementation{Name: "instructions-test", Version: "1"}, nil)
	clientSession, err := c.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()
	require.NotNil(t, clientSession.InitializeResult())
	assert.Equal(t, instructions, clientSession.InitializeResult().Instructions)
}
