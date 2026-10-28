// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureRequestID(t *testing.T) {
	t.Run("keeps an existing ID", func(t *testing.T) {
		ctx := WithRequestID(context.Background(), "req-123")
		ctx, id := EnsureRequestID(ctx)
		assert.Equal(t, "req-123", id)
		assert.Equal(t, "req-123", RequestIDFromContext(ctx))
	})

	t.Run("adds one when missing", func(t *testing.T) {
		ctx, id := EnsureRequestID(context.Background())
		assert.NotEmpty(t, id)
		assert.Equal(t, id, RequestIDFromContext(ctx))
	})

	t.Run("empty context has no ID", func(t *testing.T) {
		assert.Empty(t, RequestIDFromContext(context.Background()))
	})
}

func TestRequestIDTransport(t *testing.T) {
	var gotHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get(RequestIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	logger := log.New()
	logger.SetOutput(io.Discard)
	httpClient := CreateHTTPClient(false, logger)

	tests := []struct {
		name           string
		ctxID          string
		existingHeader string
		expected       string
	}{
		{name: "sets header from context", ctxID: "req-123", expected: "req-123"},
		{name: "no ID in context sends no header", expected: ""},
		{name: "does not overwrite a header the caller set", ctxID: "req-123", existingHeader: "caller-id", expected: "caller-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHeader = ""
			ctx := context.Background()
			if tt.ctxID != "" {
				ctx = WithRequestID(ctx, tt.ctxID)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			require.NoError(t, err)
			if tt.existingHeader != "" {
				req.Header.Set(RequestIDHeader, tt.existingHeader)
			}

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, tt.expected, gotHeader)
			if tt.existingHeader == "" {
				assert.Empty(t, req.Header.Get(RequestIDHeader), "caller's request should not be changed")
			}
		})
	}
}

func TestTerraformContextMiddlewareSetsRequestID(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	var ctxID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := TerraformContextMiddleware(logger)(next)

	t.Run("ID is in the context and the response header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.NotEmpty(t, ctxID)
		assert.Equal(t, ctxID, rr.Header().Get(RequestIDHeader))
	})

	t.Run("each request gets its own ID", func(t *testing.T) {
		rr1 := httptest.NewRecorder()
		handler.ServeHTTP(rr1, httptest.NewRequest(http.MethodPost, "/mcp", nil))
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/mcp", nil))

		assert.NotEqual(t, rr1.Header().Get(RequestIDHeader), rr2.Header().Get(RequestIDHeader))
	})

	t.Run("client supplied ID is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set(RequestIDHeader, "client-id")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.NotEqual(t, "client-id", ctxID)
		assert.NotEqual(t, "client-id", rr.Header().Get(RequestIDHeader))
	})

	t.Run("rejected requests still return an ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.Header.Set(TerraformAddress, "https://attacker.example.com")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.NotEmpty(t, rr.Header().Get(RequestIDHeader))
	})
}

// TestRequestIDSentToTerraform checks the ID the client gets back is the same one
// HCP Terraform sees, using the org allowlist's organization list call.
func TestRequestIDSentToTerraform(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	var mu sync.Mutex
	var upstreamID string
	terraformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations" {
			mu.Lock()
			upstreamID = r.Header.Get(RequestIDHeader)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, err := io.WriteString(w, `{"data":[{"id":"alpha","type":"organizations","attributes":{"name":"alpha"}}]}`)
		assert.NoError(t, err)
	}))
	t.Cleanup(terraformServer.Close)
	t.Setenv(TerraformAddress, terraformServer.URL)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := TerraformContextMiddleware(logger)(
		OrganizationAllowlistMiddleware([]string{"alpha"}, logger)(next),
	)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer allowed-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, upstreamID)
	assert.Equal(t, rr.Header().Get(RequestIDHeader), upstreamID)
}

func TestToolLoggingMiddlewareRequestID(t *testing.T) {
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "list_workspaces"}}

	t.Run("logs the ID from the context and passes it on", func(t *testing.T) {
		var buf bytes.Buffer
		logger := log.New()
		logger.SetOutput(&buf)

		var nextID string
		handler := ToolLoggingMiddleware(logger)(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			nextID = RequestIDFromContext(ctx)
			return mcp.NewToolResultText("ok"), nil
		})

		_, err := handler(WithRequestID(context.Background(), "req-123"), request)
		require.NoError(t, err)
		assert.Equal(t, "req-123", nextID)
		assert.Contains(t, buf.String(), "request_id=req-123")
	})

	t.Run("adds an ID when there isn't one, like in stdio mode", func(t *testing.T) {
		var buf bytes.Buffer
		logger := log.New()
		logger.SetOutput(&buf)

		var nextID string
		handler := ToolLoggingMiddleware(logger)(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			nextID = RequestIDFromContext(ctx)
			return mcp.NewToolResultText("ok"), nil
		})

		_, err := handler(context.Background(), request)
		require.NoError(t, err)
		require.NotEmpty(t, nextID)
		assert.Contains(t, buf.String(), "request_id="+nextID)
	})
}
