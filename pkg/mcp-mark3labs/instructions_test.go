// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/instructions"
	mcpofficial "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestLegacyServerInitializationIncludesSharedInstructions(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	server, _ := NewServer("test", logger, toolsets.NewToolsetFilter([]string{toolsets.Search}))
	response := server.HandleMessage(context.Background(), json.RawMessage(`{
		"jsonrpc":"2.0","id":1,"method":"initialize",
		"params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}
	}`))
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var wire struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Equal(t, instructions.Text, wire.Result.Instructions, string(encoded))
}

func TestOfficialServerInitializationIncludesSharedInstructions(t *testing.T) {
	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := mcpofficial.NewServer("test", instructions.Text, 0, slog.Default(), toolsets.NewToolsetFilter([]string{toolsets.Search}))
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "instructions-test", Version: "0"}, nil)
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	session, err := client.Connect(connectCtx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	require.Equal(t, instructions.Text, session.InitializeResult().Instructions)
}
