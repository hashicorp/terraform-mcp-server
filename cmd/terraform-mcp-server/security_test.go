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
			name:           "development mode - localhost allowed",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true,
		},
		{
			name:           "development mode - 127.0.0.1 allowed",
			origin:         "http://127.0.0.1:3000",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       true,
		},
		{
			name:           "development mode - disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "development",
			expected:       false,
		},
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

	// Test default values
	os.Unsetenv("MCP_ALLOWED_ORIGINS")
	os.Unsetenv("MCP_CORS_MODE")
	config := LoadCORSConfigFromEnv()
	assert.Equal(t, "strict", config.Mode)
	assert.Empty(t, config.AllowedOrigins)

	// Test with values set
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
		{
			name:           "allowed origin",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK,
			expectedHeader: true,
		},
		{
			name:           "disallowed origin",
			origin:         "https://evil.com",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusForbidden,
			expectedHeader: false,
		},
		{
			name:           "no origin header",
			origin:         "",
			allowedOrigins: []string{"https://example.com"},
			mode:           "strict",
			expectedStatus: http.StatusOK,
			expectedHeader: false,
		},
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

	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Mock handler should not be called for OPTIONS request")
	})

	handler := NewSecurityHandler(mockHandler, []string{"https://example.com"}, "strict", logger)
	
	req := httptest.NewRequest("OPTIONS", "/mcp", nil)
	req.Header.Set("Origin", "https://example.com")
	
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Methods"))
}
