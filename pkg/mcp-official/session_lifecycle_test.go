// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0
package mcpofficial

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestWithSessionLifecycleCreatesAndCleansUpClients(t *testing.T) {
	// A fake TFE API so client creation doesn't touch the network
	tfe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("TFP-API-Version", "2.6")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer tfe.Close()
	t.Setenv("TFE_ADDRESS", tfe.URL)
	t.Setenv("TFE_TOKEN", "test-token")

	const sessionID = "lifecycle-test"
	officialclient.SetFallbackSessionID(sessionID)
	t.Cleanup(func() { officialclient.SetFallbackSessionID("") })

	logger := log.New()
	logger.SetOutput(io.Discard)
	rateLimiter := client.NewRateLimitMiddleware(client.LoadRateLimitConfigFromEnv(), logger)

	srv := NewServer("test", "", 0, slog.New(slog.NewTextHandler(io.Discard, nil)),
		toolsets.NewToolsetFilter([]string{toolsets.Registry}),
		WithSessionLifecycle(rateLimiter, logger))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The SDK's own client handshakes with server/discover, which doesn't fire
	// the initialized hook
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	go func() { _, _ = io.Copy(io.Discard, clientIn) }() // drain server replies
	ss, err := srv.Connect(ctx, &mcp.IOTransport{Reader: serverIn, Writer: serverOut}, nil)
	require.NoError(t, err)
	defer ss.Close()

	_, err = io.WriteString(clientOut,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"0"}}}`+"\n"+
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool { return client.GetTfeClient(sessionID) != nil },
		5*time.Second, 20*time.Millisecond, "client should be created when the session starts")

	require.NoError(t, clientOut.Close()) // client disconnects
	require.Eventually(t, func() bool { return client.GetTfeClient(sessionID) == nil },
		5*time.Second, 20*time.Millisecond, "client should be removed when the session ends")
}
