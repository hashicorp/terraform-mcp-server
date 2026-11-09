// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
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

// statusRecorder keeps the HTTP status of each POST, so the last one is the
// status for the most recent CallTool.
type statusRecorder struct {
	mu       sync.Mutex
	statuses []int
}

func (s *statusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && req.Method == http.MethodPost {
		s.mu.Lock()
		s.statuses = append(s.statuses, resp.StatusCode)
		s.mu.Unlock()
	}
	return resp, err
}

func (s *statusRecorder) last() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.statuses) == 0 {
		return 0
	}
	return s.statuses[len(s.statuses)-1]
}

// TestAPIErrorsThroughSDK runs tool calls through a real go-sdk server and
// client with APIErrors on. API errors should come back as a normal 200 with
// the code in _meta, so the client gets the result, the call isn't replayed,
// and the session still works after.
func TestAPIErrorsThroughSDK(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	server.AddReceivingMiddleware(middleware.APIErrors())

	var expiredCalls atomic.Int32
	type noArgs struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "expired_token"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		expiredCalls.Add(1)
		return nil, nil, fmt.Errorf("failed to read run run-1: %w", tfe.ErrUnauthorized)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "missing_run"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return nil, nil, fmt.Errorf("failed to read run run-1: %w", tfe.ErrResourceNotFound)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "works"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true})
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	recorder := &statusRecorder{}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL,
		HTTPClient:           &http.Client{Transport: recorder},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	t.Run("not found is a 200 with the code in _meta", func(t *testing.T) {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "missing_run"})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Equal(t, middleware.APIErrorNotFound, result.Meta[middleware.MetaErrorCode])
		assert.Equal(t, http.StatusOK, recorder.last())
	})

	t.Run("expired token is a 200 with the code in _meta", func(t *testing.T) {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "expired_token"})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Equal(t, middleware.APIErrorUnauthorized, result.Meta[middleware.MetaErrorCode])
		assert.EqualValues(t, http.StatusUnauthorized, result.Meta[middleware.MetaHTTPStatus])
		assert.Equal(t, http.StatusOK, recorder.last())
		assert.Equal(t, int32(1), expiredCalls.Load(), "the tool call shouldn't be replayed")
	})

	t.Run("session still works after an api error", func(t *testing.T) {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "works"})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, http.StatusOK, recorder.last())
	})
}
