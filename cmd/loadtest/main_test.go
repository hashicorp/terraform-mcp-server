// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpServer starts a real MCP server with three tools: ok succeeds, fail
// returns isError, and hang blocks until the call is canceled.
func mcpServer(t *testing.T) string {
	t.Helper()
	// hang also stops when the test cleans up, since the server doesn't cancel
	// the handler when the client gives up and Close waits for it
	release := make(chan struct{})
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ok"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "fail"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not found"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "hang"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		select {
		case <-ctx.Done():
		case <-release:
		}
		return &mcp.CallToolResult{}, nil, nil
	})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	t.Cleanup(func() { close(release); ts.Close() })
	return ts.URL
}

// stalledServer accepts connections but never answers, like a server that's
// up but stuck before finishing the MCP handshake.
func stalledServer(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); ts.Close() })
	return ts.URL
}

func run(t *testing.T, endpoint, tool string, duration, timeout time.Duration) *loadResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	return runLoad(ctx, loadConfig{
		endpoint:   endpoint,
		httpClient: http.DefaultClient,
		workers:    2,
		qps:        20,
		timeout:    timeout,
		tool:       tool,
		args:       json.RawMessage(`{}`),
	})
}

func TestRunLoad(t *testing.T) {
	t.Run("successful calls pass", func(t *testing.T) {
		r := run(t, mcpServer(t), "ok", time.Second, time.Second)
		assert.Positive(t, r.success)
		assert.Zero(t, r.failure)
		assert.Len(t, r.successLatencies, int(r.success))
		assert.False(t, r.failed())
	})

	t.Run("tool errors count as failures", func(t *testing.T) {
		r := run(t, mcpServer(t), "fail", time.Second, time.Second)
		assert.Zero(t, r.success)
		assert.Positive(t, r.failure)
		assert.True(t, r.failed())
	})

	t.Run("stalled handshake counts as a connect error", func(t *testing.T) {
		r := run(t, stalledServer(t), "ok", 2*time.Second, 300*time.Millisecond)
		assert.EqualValues(t, 2, r.connectFailures, "both workers should time out connecting")
		assert.Zero(t, r.total())
		assert.True(t, r.failed())
	})

	t.Run("stalled handshake still fails when duration is shorter than the timeout", func(t *testing.T) {
		r := run(t, stalledServer(t), "ok", 300*time.Millisecond, 5*time.Second)
		assert.Zero(t, r.connectFailures, "the run ended before connect timed out")
		assert.Zero(t, r.total())
		assert.True(t, r.failed(), "a run with no completed calls must not pass")
	})

	t.Run("calls that hang until the run ends fail", func(t *testing.T) {
		r := run(t, mcpServer(t), "hang", 500*time.Millisecond, 5*time.Second)
		assert.Zero(t, r.connectFailures)
		assert.Zero(t, r.total())
		assert.True(t, r.failed())
	})

	t.Run("nothing listening counts as a connect error", func(t *testing.T) {
		ts := httptest.NewServer(http.NotFoundHandler())
		endpoint := ts.URL
		ts.Close()
		r := run(t, endpoint, "ok", time.Second, time.Second)
		assert.EqualValues(t, 2, r.connectFailures)
		assert.True(t, r.failed())
	})
}

func TestValidateEndpoint(t *testing.T) {
	for _, endpoint := range []string{"http://localhost:8080/mcp", "https://mcp.example.com/mcp"} {
		require.NoError(t, validateEndpoint(endpoint), endpoint)
	}
	for _, endpoint := range []string{"localhost:8080/mcp", "ftp://example.com/mcp", "http:///mcp", "http://%zz"} {
		require.Error(t, validateEndpoint(endpoint), endpoint)
	}
}
