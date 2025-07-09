// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		mode           string
		expected       bool
	}{
		// Test case: In strict mode, an origin that matches one in the allowed list should be allowed
		{
			name:           "strict mode - allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			mode:           "strict",
			expected:       true,
		},
		// Test case: In strict mode, an origin that doesn't match any in the allowed list should be rejected
		{
			name:           "strict mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			mode:           "strict",
			expected:       false,
		},
		// Test case: In development mode, localhost origins should be allowed regardless of the allowed list
		{
			name:           "development mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true,
		},
		// Test case: In development mode, 127.0.0.1 (IPv4 localhost) should be allowed
		{
			name:           "development mode - 127.0.0.1 allowed",
			origin:         "http://127.0.0.1:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true,
		},
		// Test case: In development mode, non-localhost origins not in the allowed list should be rejected
		{
			name:           "development mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       false,
		},
		// Test case: In disabled mode, all origins should be allowed regardless of the allowed list
		{
			name:           "disabled mode - all origins allowed",
			origin:         "https://evil.com",
			allowedOrigins: []string{},
			mode:           "disabled",
			expected:       true,
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
		// Test case: Request with an allowed origin in strict mode should be accepted
		// and CORS headers should be set in the response
		{
			name:           "allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK,
			expectedHeader: true,
		},
		// Test case: Request with a disallowed origin in strict mode should be rejected
		// with a 403 Forbidden status and no CORS headers
		{
			name:           "disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusForbidden,
			expectedHeader: false,
		},
		// Test case: Request with no Origin header in strict mode should be allowed
		// This is important because it means requests without Origin headers bypass CORS checks
		{
			name:           "no origin header",
			origin:         "",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK,
			expectedHeader: false,
		},
		// Test case: Duplicate of above to explicitly test the behavior of missing Origin headers
		// in strict mode. Requests without Origin headers are allowed regardless of CORS settings.
		// This is why localhost requests might work even in strict mode if they don't include an Origin header.
		{
			name:           "strict mode - no origin header",
			origin:         "", // No origin header
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK, // Should be allowed
			expectedHeader: false,
		},
		// Test case: Request with a localhost origin in strict mode should be rejected
		// This confirms that localhost is not automatically allowed in strict mode
		{
			name:           "strict mode - localhost origin",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusForbidden, // Should be rejected
			expectedHeader: false,
		},
		// Test case: Request with a localhost origin in development mode should be allowed
		// and CORS headers should be set in the response
		{
			name:           "development mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			mode:           "development",
			expectedStatus: http.StatusOK,
			expectedHeader: true,
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
