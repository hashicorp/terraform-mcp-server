// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldUseStatelessMode(t *testing.T) {
	// Save original env var to restore later
	origMode := os.Getenv("MCP_SESSION_MODE")
	defer func() {
		os.Setenv("MCP_SESSION_MODE", origMode)
	}()

	// Test case: When MCP_SESSION_MODE is not set, stateful mode should be used (default)
	os.Unsetenv("MCP_SESSION_MODE")
	assert.False(t, shouldUseStatelessMode(), "Stateful mode should be used when MCP_SESSION_MODE is not set")

	// Test case: When MCP_SESSION_MODE is set to "stateful", stateful mode should be used
	os.Setenv("MCP_SESSION_MODE", "stateful")
	assert.False(t, shouldUseStatelessMode(), "Stateful mode should be used when MCP_SESSION_MODE is set to 'stateful'")

	// Test case: When MCP_SESSION_MODE is set to "stateless", stateless mode should be used
	os.Setenv("MCP_SESSION_MODE", "stateless")
	assert.True(t, shouldUseStatelessMode(), "Stateless mode should be used when MCP_SESSION_MODE is set to 'stateless'")

	// Test case: Case insensitivity - uppercase
	os.Setenv("MCP_SESSION_MODE", "STATELESS")
	assert.True(t, shouldUseStatelessMode(), "Stateless mode should be used when MCP_SESSION_MODE is set to 'STATELESS' (uppercase)")

	// Test case: Case insensitivity - mixed case
	os.Setenv("MCP_SESSION_MODE", "StAtElEsS")
	assert.True(t, shouldUseStatelessMode(), "Stateless mode should be used when MCP_SESSION_MODE is set to 'StAtElEsS' (mixed case)")

	// Test case: Invalid value should default to stateful mode
	os.Setenv("MCP_SESSION_MODE", "invalid-value")
	assert.False(t, shouldUseStatelessMode(), "Stateful mode should be used when MCP_SESSION_MODE is set to an invalid value")
}

func TestStripSessionIDMiddleware(t *testing.T) {
	var gotSessionID string
	nextCalled := false
	handler := stripSessionIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		gotSessionID = r.Header.Get("Mcp-Session-Id")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Mcp-Session-Id", strings.Repeat("a", 60<<10))
	req.Header.Set("Authorization", "Bearer user-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, gotSessionID, "session ID should be stripped in stateless mode")
	assert.Equal(t, "Bearer user-token", req.Header.Get("Authorization"), "other headers should be left alone")
}

func TestShouldDisableLocalhostProtection(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "unset", value: "", expected: false},
		{name: "true", value: "true", expected: true},
		{name: "uppercase", value: "TRUE", expected: true},
		{name: "false", value: "false", expected: false},
		{name: "invalid", value: "yes", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MCP_DISABLE_LOCALHOST_PROTECTION", tt.value)
			assert.Equal(t, tt.expected, shouldDisableLocalhostProtection())
		})
	}
}

func TestGetOfficialStreamableServer_LocalhostProtection(t *testing.T) {
	tests := []struct {
		name           string
		disable        string
		host           string
		expectedStatus int
	}{
		{name: "localhost allowed", host: "", expectedStatus: http.StatusOK},
		{name: "non-localhost Host rejected", host: "proxy.example.com", expectedStatus: http.StatusForbidden},
		{name: "non-localhost Host allowed when disabled", disable: "true", host: "proxy.example.com", expectedStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MCP_DISABLE_LOCALHOST_PROTECTION", tt.disable)
			logger := metricsTestLogger()

			handler := getOfficialStreamableServer(
				context.Background(),
				0,    // heartbeatInterval
				true, // isStateless
				client.CORSConfig{Mode: "disabled"},
				logger,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				nil, // organizationAllowlist
				toolsets.NewToolsetFilter([]string{"list_workspaces"}),
				client.NewRateLimitMiddleware(client.DefaultRateLimitConfig(), logger),
				client.MetricsConfig{Enabled: false},
			)

			// httptest listens on 127.0.0.1, so requests arrive on a loopback connection
			srv := httptest.NewServer(handler)
			defer srv.Close()

			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
			req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if tt.host != "" {
				req.Host = tt.host
			}

			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}
