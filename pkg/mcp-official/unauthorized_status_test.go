// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	unauthorizedResult = `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"unauthorized: failed to read run run-1: unauthorized"}],"isError":true,"_meta":{"error_code":"unauthorized","http_status":401}}}`
	notFoundResult     = `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"not_found: failed to read run run-1: resource not found"}],"isError":true,"_meta":{"error_code":"not_found","http_status":404}}}`
	successResult      = `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}]}}`
	progressEvent      = `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":1,"progress":1}}`
)

func sseEvent(data string) string {
	return "event: message\ndata: " + data + "\n\n"
}

func TestUnauthorizedStatus(t *testing.T) {
	tests := []struct {
		name       string
		header     int
		writes     []string
		wantStatus int
		wantAuth   bool
	}{
		{name: "unauthorized sse result", writes: []string{sseEvent(unauthorizedResult)}, wantStatus: http.StatusUnauthorized, wantAuth: true},
		{name: "unauthorized json result", writes: []string{unauthorizedResult}, wantStatus: http.StatusUnauthorized, wantAuth: true},
		{name: "explicit 200 can still become 401", header: http.StatusOK, writes: []string{unauthorizedResult}, wantStatus: http.StatusUnauthorized, wantAuth: true},
		{name: "not found stays 200", writes: []string{sseEvent(notFoundResult)}, wantStatus: http.StatusOK},
		{name: "success stays 200", writes: []string{sseEvent(successResult)}, wantStatus: http.StatusOK},
		{name: "explicit 200 with success", header: http.StatusOK, writes: []string{successResult}, wantStatus: http.StatusOK},
		{name: "status already sent when something streams first", writes: []string{sseEvent(progressEvent), sseEvent(unauthorizedResult)}, wantStatus: http.StatusOK},
		{name: "other statuses pass through", header: http.StatusAccepted, wantStatus: http.StatusAccepted},
		{name: "garbage stays 200", writes: []string{"not an event"}, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := UnauthorizedStatus(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.header != 0 {
					w.WriteHeader(tt.header)
				}
				for _, write := range tt.writes {
					_, err := w.Write([]byte(write))
					require.NoError(t, err)
				}
			}))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp/official", nil))

			assert.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantAuth {
				assert.Equal(t, `Bearer error="invalid_token"`, rr.Header().Get("WWW-Authenticate"))
			} else {
				assert.Empty(t, rr.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

// statusRecorder records the HTTP status of every POST response the client
// gets back, so after a CallTool the last one is that call's.
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

// TestUnauthorizedStatusThroughSDK runs tool calls through the go-sdk
// server and streamable HTTP handler with APIErrors and UnauthorizedStatus
// wired in, the same way the official HTTP server is.
func TestUnauthorizedStatusThroughSDK(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	server.AddReceivingMiddleware(middleware.APIErrors())

	type noArgs struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "expired_token"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return nil, nil, fmt.Errorf("failed to read run run-1: %w", tfe.ErrUnauthorized)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "missing_run"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return nil, nil, fmt.Errorf("failed to read run run-1: %w", tfe.ErrResourceNotFound)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "works"}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true})
	httpServer := httptest.NewServer(UnauthorizedStatus(handler))
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

	t.Run("success is a 200", func(t *testing.T) {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "works"})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, http.StatusOK, recorder.last())
	})

	t.Run("not found is a 200 with the code in _meta", func(t *testing.T) {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "missing_run"})
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Equal(t, middleware.APIErrorNotFound, result.Meta[middleware.MetaErrorCode])
		assert.Equal(t, http.StatusOK, recorder.last())
	})

	t.Run("expired token is a 401", func(t *testing.T) {
		_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "expired_token"})
		assert.Equal(t, http.StatusUnauthorized, recorder.last())
	})
}
