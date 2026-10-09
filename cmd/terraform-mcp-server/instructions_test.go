// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	mcpofficial "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools/search/importworkflow"
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
		"Local validation does not replace the target workspace's plan",
		"**Search-to-Import (when Search tools are enabled)**:",
		"`get_query_summary`", "Explicitly", "select up to 100", "keep the complete response", "`identity_support`",
		"Search provider version, observations, and generated HCL are source",
		"**authoring directory**", "Keep the archive root exactly as downloaded", "explicit, reviewed file list", "re-check it for\n   key and credential files", "git work tree", "already holds the user's Terraform files is fine", "not supported yet: stop", "takes no local path", "without asking the user first", "`terraform init -backend=false`", "do not run `init` for a different\n   provider version", "not fixed rules", "list\n   each adaptation", "exactly 100 results", "MCP does not download, edit, or upload archive bytes",
		"`get_import_configuration_download`", "`create_import_cv`", "`create_import_run`", "`verify_import_plan`",
		"after the single user review described", "uncertain create must be reconciled",
		"full finished plan", "refresh drift", "A plan does not", "separate review and approval",
	} {
		assert.Contains(t, instructions, guidance)
	}
	assert.NotContains(t, instructions, "explicit confirmation for each speculative")
	assert.NotContains(t, instructions, "Query registries for latest provider/module versions")
	assert.NotContains(t, instructions, "public as fallback")
}

func TestServerInstructionsScopeTargetChoiceToImportIntent(t *testing.T) {
	source := strings.Index(instructions, "source A); ask and wait")
	discovery := strings.Index(instructions, "Discover the relevant Search provider/list-resource schema")
	intent := strings.Index(instructions, "Only if the user wants to import")
	choice := strings.Index(instructions, "Import into the same workspace that ran Search (A), or designate a")
	prepare := strings.Index(instructions, "`prepare_import` with the selection")
	require.NotEqual(t, -1, source)
	require.NotEqual(t, -1, discovery)
	require.NotEqual(t, -1, intent)
	require.NotEqual(t, -1, choice)
	require.NotEqual(t, -1, prepare)
	assert.Less(t, source, discovery, "choose the Search source before looking up provider schemas")
	assert.Less(t, discovery, intent, "a Search-only request does not ask for a target")
	assert.Less(t, intent, choice, "ask for a target only on import intent")
	assert.Less(t, choice, prepare, "offer the same-workspace choice before target preparation")
	assert.Contains(t, instructions, "user instead supplies an existing QueryRun ID")
	assert.Contains(t, instructions, "stop after reporting results without asking for a target")
	assert.Contains(t, instructions, "same **only when chosen explicitly**")
	assert.Contains(t, instructions, "do not advertise a multi-target workflow")
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
			assert.Contains(t, result.Instructions, "source A); ask and wait")
			assert.Contains(t, result.Instructions, "Only if the user wants to import")
			assert.Contains(t, result.Instructions, "same workspace that ran Search (A)")
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
	assert.Contains(t, clientSession.InitializeResult().Instructions, "Only if the user wants to import")
}

func TestServerInstructionsUseSharedImportVocabulary(t *testing.T) {
	lower := strings.ToLower(instructions)
	for _, phrase := range importworkflow.RetiredImportPhrases() {
		assert.NotContains(t, lower, phrase, "instructions use retired wording")
	}
	assert.NotContains(t, lower, "route 1")
	assert.NotContains(t, lower, "route 2")
}
