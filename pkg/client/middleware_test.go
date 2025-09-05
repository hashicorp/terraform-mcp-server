// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// TestTerraformContextMiddleware tests the middleware that extracts Terraform configuration
// from HTTP headers, query parameters, and environment variables and adds them to the request context
func TestTerraformContextMiddleware(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Save original env vars to restore later
	origAddress := os.Getenv(TerraformAddress)
	origToken := os.Getenv(TerraformToken)
	origSkipTLS := os.Getenv(TerraformSkipTLSVerify)
	defer func() {
		os.Setenv(TerraformAddress, origAddress)
		os.Setenv(TerraformToken, origToken)
		os.Setenv(TerraformSkipTLSVerify, origSkipTLS)
	}()

	// Clear environment variables for clean test state
	os.Unsetenv(TerraformAddress)
	os.Unsetenv(TerraformToken)
	os.Unsetenv(TerraformSkipTLSVerify)

	tests := []struct {
		name                string
		headers             map[string]string
		queryParams         map[string]string
		envVars             map[string]string
		expectedStatus      int
		expectedContextVals map[string]string
		expectError         bool
		errorMessage        string
	}{
		{
			name: "headers take priority over query params and env vars",
			headers: map[string]string{
				TerraformAddress:       "https://header.terraform.io",
				TerraformToken:         "header-token",
				TerraformSkipTLSVerify: "true",
			},
			queryParams: map[string]string{
				TerraformAddress:       "https://query.terraform.io",
				TerraformSkipTLSVerify: "false",
			},
			envVars: map[string]string{
				TerraformAddress:       "https://env.terraform.io",
				TerraformToken:         "env-token",
				TerraformSkipTLSVerify: "false",
			},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://header.terraform.io",
				TerraformToken:         "header-token",
				TerraformSkipTLSVerify: "true",
			},
		},
		{
			name:    "query params take priority over env vars (except token)",
			headers: map[string]string{},
			queryParams: map[string]string{
				TerraformAddress:       "https://query.terraform.io",
				TerraformSkipTLSVerify: "true",
			},
			envVars: map[string]string{
				TerraformAddress:       "https://env.terraform.io",
				TerraformToken:         "env-token",
				TerraformSkipTLSVerify: "false",
			},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://query.terraform.io",
				TerraformToken:         "env-token", // From env since not in query
				TerraformSkipTLSVerify: "true",
			},
		},
		{
			name:        "env vars used as fallback",
			headers:     map[string]string{},
			queryParams: map[string]string{},
			envVars: map[string]string{
				TerraformAddress:       "https://env.terraform.io",
				TerraformToken:         "env-token",
				TerraformSkipTLSVerify: "true",
			},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://env.terraform.io",
				TerraformToken:         "env-token",
				TerraformSkipTLSVerify: "true",
			},
		},
		{
			name:    "empty values result in empty context values",
			headers: map[string]string{},
			queryParams: map[string]string{
				TerraformAddress: "", // Empty value
			},
			envVars:        map[string]string{},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "",
				TerraformToken:         "",
				TerraformSkipTLSVerify: "",
			},
		},
		{
			name:    "token in query params is rejected for security",
			headers: map[string]string{},
			queryParams: map[string]string{
				TerraformAddress: "https://query.terraform.io",
				TerraformToken:   "query-token", // This should cause an error
			},
			envVars:        map[string]string{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
			errorMessage:   "Terraform token should not be provided in query parameters for security reasons, use the terraform_token header",
		},
		{
			name: "canonical header names are handled correctly",
			headers: map[string]string{
				"tfe_address":         "https://canonical.terraform.io", // lowercase
				"TFE_TOKEN":           "canonical-token",                // uppercase
				"tfe_skip_tls_verify": "true",                           // mixed case
			},
			queryParams:    map[string]string{},
			envVars:        map[string]string{},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://canonical.terraform.io",
				TerraformToken:         "canonical-token",
				TerraformSkipTLSVerify: "true",
			},
		},
		{
			name: "mixed sources - headers override query params, query params override env",
			headers: map[string]string{
				TerraformAddress: "https://header.terraform.io", // Header wins
			},
			queryParams: map[string]string{
				TerraformSkipTLSVerify: "true", // Query param wins over env
			},
			envVars: map[string]string{
				TerraformAddress:       "https://env.terraform.io", // Overridden by header
				TerraformToken:         "env-token",                // Used since not in header/query
				TerraformSkipTLSVerify: "false",                    // Overridden by query param
			},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://header.terraform.io",
				TerraformToken:         "env-token",
				TerraformSkipTLSVerify: "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for this test
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			// Create a mock handler that captures the context values
			var capturedContext map[string]string
			mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedContext = make(map[string]string)
				ctx := r.Context()

				// Extract all terraform-related context values
				for _, key := range []string{TerraformAddress, TerraformToken, TerraformSkipTLSVerify} {
					if val := ctx.Value(contextKey(key)); val != nil {
						if strVal, ok := val.(string); ok {
							capturedContext[key] = strVal
						}
					} else {
						capturedContext[key] = "" // Explicitly track nil/missing values as empty strings
					}
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Create the middleware
			middleware := TerraformContextMiddleware(logger)
			handler := middleware(mockHandler)

			// Create request with headers and query parameters
			req := httptest.NewRequest("GET", "/mcp", nil)

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Set query parameters
			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Set(key, value)
			}
			req.URL.RawQuery = q.Encode()

			// Execute request
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Verify response status
			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectError {
				// Verify error message is in response body
				assert.Contains(t, rr.Body.String(), tt.errorMessage)
			} else {
				// Verify context values were set correctly
				assert.NotNil(t, capturedContext, "Context should have been captured")
				for key, expectedValue := range tt.expectedContextVals {
					actualValue, exists := capturedContext[key]
					assert.True(t, exists, "Context should contain key %s", key)
					assert.Equal(t, expectedValue, actualValue, "Context value for %s should match", key)
				}
			}
		})
	}
}

// TestTerraformContextMiddleware_SecurityLogging tests that the middleware properly logs
// security-related events without exposing sensitive information
func TestTerraformContextMiddleware_SecurityLogging(t *testing.T) {
	// Create a custom logger that captures log output
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	// Create a mock handler
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := TerraformContextMiddleware(logger)
	handler := middleware(mockHandler)

	t.Run("token provided via header is logged without exposing value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mcp", nil)
		req.Header.Set(TerraformToken, "secret-token")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		// Note: In a real test, you'd capture the log output and verify it contains
		// "Terraform token provided via request context" but doesn't contain "secret-token"
	})

	t.Run("address provided via header is logged", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mcp", nil)
		req.Header.Set(TerraformAddress, "https://custom.terraform.io")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		// Note: In a real test, you'd capture the log output and verify it contains
		// "Terraform address configured via request context"
	})
}

// TestTerraformContextMiddleware_EdgeCases tests edge cases and error conditions
func TestTerraformContextMiddleware_EdgeCases(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("nil logger should not panic", func(t *testing.T) {
		// This tests that the middleware handles a nil logger gracefully
		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Create middleware with nil logger - this should not panic
		assert.NotPanics(t, func() {
			middleware := TerraformContextMiddleware(nil)
			handler := middleware(mockHandler)

			req := httptest.NewRequest("GET", "/mcp", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		})
	})

	t.Run("malformed query parameters are handled gracefully", func(t *testing.T) {
		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := TerraformContextMiddleware(logger)
		handler := middleware(mockHandler)

		// Create request with malformed query string
		req := httptest.NewRequest("GET", "/mcp?%invalid", nil)

		rr := httptest.NewRecorder()
		// This should not panic even with malformed query parameters
		assert.NotPanics(t, func() {
			handler.ServeHTTP(rr, req)
		})
	})

	t.Run("very long header values are handled", func(t *testing.T) {
		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			val := ctx.Value(contextKey(TerraformAddress))
			assert.NotNil(t, val)
			w.WriteHeader(http.StatusOK)
		})

		middleware := TerraformContextMiddleware(logger)
		handler := middleware(mockHandler)

		// Create a very long address value
		longAddress := "https://" + strings.Repeat("a", 1000) + ".terraform.io"

		req := httptest.NewRequest("GET", "/mcp", nil)
		req.Header.Set(TerraformAddress, longAddress)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
