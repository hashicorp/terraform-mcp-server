// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestIsOriginAllowed tests the core function that determines if an origin is allowed
// based on the CORS configuration. This function is called by the security handler
// when processing requests with Origin headers.
func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		mode           string
		expected       bool
	}{
		// Strict mode tests
		{
			name:           "strict mode - allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			mode:           "strict",
			expected:       true,
		},
		{
			name:           "strict mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			mode:           "strict",
			expected:       false,
		},
		{
			name:           "strict mode - localhost origin",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expected:       false, // Localhost is not automatically allowed in strict mode
		},
		// Note: The "no origin header" case cannot be directly tested here since
		// isOriginAllowed requires an origin parameter. This behavior is tested
		// in TestSecurityHandler instead.

		// Development mode tests
		{
			name:           "development mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true, // Localhost is automatically allowed in development mode
		},
		{
			name:           "development mode - 127.0.0.1 allowed",
			origin:         "http://127.0.0.1:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true, // IPv4 localhost is automatically allowed in development mode
		},
		{
			name:           "development mode - ::1 allowed",
			origin:         "http://[::1]:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true, // IPv6 localhost is automatically allowed in development mode
		},
		{
			name:           "development mode - allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true, // Explicitly allowed origins are still allowed in development mode
		},
		{
			name:           "development mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       false, // Non-localhost, non-allowed origins are still rejected in development mode
		},

		// Disabled mode tests
		{
			name:           "disabled mode - any origin allowed",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "disabled",
			expected:       true, // All origins are allowed in disabled mode
		},
		{
			name:           "disabled mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			mode:           "disabled",
			expected:       true, // Localhost is allowed in disabled mode (like any origin)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOriginAllowed(tt.origin, tt.allowedOrigins, tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadCORSConfigFromEnv(t *testing.T) {
	// Save original env vars to restore later
	origOrigins := os.Getenv("MCP_ALLOWED_ORIGINS")
	origMode := os.Getenv("MCP_CORS_MODE")
	defer func() {
		os.Setenv("MCP_ALLOWED_ORIGINS", origOrigins)
		os.Setenv("MCP_CORS_MODE", origMode)
	}()

	// Test case: When environment variables are not set, default values should be used
	// Default mode should be "strict" and allowed origins should be empty
	os.Unsetenv("MCP_ALLOWED_ORIGINS")
	os.Unsetenv("MCP_CORS_MODE")
	config := LoadCORSConfigFromEnv()
	assert.Equal(t, "strict", config.Mode)
	assert.Empty(t, config.AllowedOrigins)

	// Test case: When environment variables are set, their values should be used
	// Mode should be "development" and allowed origins should contain the specified values
	os.Setenv("MCP_ALLOWED_ORIGINS", "https://example.com, https://test.com")
	os.Setenv("MCP_CORS_MODE", "development")
	config = LoadCORSConfigFromEnv()
	assert.Equal(t, "development", config.Mode)
	assert.Equal(t, []string{"https://example.com", "https://test.com"}, config.AllowedOrigins)
}

// TestSecurityHandler tests the HTTP handler that applies CORS validation logic
// to incoming requests. This test verifies the complete request handling flow,
// including origin validation and response generation.
func TestSecurityHandler(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Create a mock handler that always succeeds
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		mode           string
		expectedStatus int
		expectedHeader bool
	}{
		// Strict mode tests
		{
			name:           "strict mode - allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK,
			expectedHeader: true, // CORS headers should be set for allowed origins
		},
		{
			name:           "strict mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusForbidden,
			expectedHeader: false, // No CORS headers for rejected requests
		},
		{
			name:           "strict mode - localhost origin",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusForbidden, // Localhost is not automatically allowed in strict mode
			expectedHeader: false,
		},
		{
			name:           "strict mode - no origin header",
			origin:         "", // No origin header
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK, // Requests without Origin headers bypass CORS checks
			expectedHeader: false,         // No CORS headers when no Origin header is present
		},

		// Development mode tests
		{
			name:           "development mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			mode:           "development",
			expectedStatus: http.StatusOK, // Localhost is automatically allowed in development mode
			expectedHeader: true,          // CORS headers should be set
		},
		{
			name:           "development mode - 127.0.0.1 allowed",
			origin:         "http://127.0.0.1:3000",
			allowedOrigins: []string{},
			mode:           "development",
			expectedStatus: http.StatusOK, // IPv4 localhost is automatically allowed in development mode
			expectedHeader: true,
		},
		{
			name:           "development mode - allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expectedStatus: http.StatusOK, // Explicitly allowed origins are still allowed in development mode
			expectedHeader: true,
		},
		{
			name:           "development mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expectedStatus: http.StatusForbidden, // Non-localhost, non-allowed origins are still rejected
			expectedHeader: false,
		},
		{
			name:           "development mode - no origin header",
			origin:         "", // No origin header
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expectedStatus: http.StatusOK, // Requests without Origin headers bypass CORS checks
			expectedHeader: false,
		},

		// Disabled mode tests
		{
			name:           "disabled mode - any origin allowed",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "disabled",
			expectedStatus: http.StatusOK, // All origins are allowed in disabled mode
			expectedHeader: true,
		},
		{
			name:           "disabled mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			mode:           "disabled",
			expectedStatus: http.StatusOK, // Localhost is allowed in disabled mode (like any origin)
			expectedHeader: true,
		},
		{
			name:           "disabled mode - no origin header",
			origin:         "", // No origin header
			allowedOrigins: []string{},
			mode:           "disabled",
			expectedStatus: http.StatusOK, // Requests without Origin headers are allowed
			expectedHeader: false,         // No CORS headers when no Origin header is present
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewSecurityHandler(mockHandler, tt.allowedOrigins, tt.mode, logger)

			req := httptest.NewRequest("GET", "/mcp", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedHeader {
				assert.Equal(t, tt.origin, rr.Header().Get("Access-Control-Allow-Origin"))
				assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Methods"))
			} else if tt.expectedStatus == http.StatusOK {
				assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestOptionsRequest tests the handling of CORS preflight requests (OPTIONS method)
// which are handled specially by the security handler.
func TestOptionsRequest(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Create a mock handler that fails the test if called
	// This tests that OPTIONS requests are handled by the security handler
	// and not passed to the wrapped handler
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Mock handler should not be called for OPTIONS request")
	})

	// Test case: OPTIONS request (CORS preflight) should be handled by the security handler
	// and should return 200 OK with appropriate CORS headers
	handler := NewSecurityHandler(mockHandler, []string{"https://example.com"}, "strict", logger)

	req := httptest.NewRequest("OPTIONS", "/mcp", nil)
	req.Header.Set("Origin", "https://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Methods"))
}
