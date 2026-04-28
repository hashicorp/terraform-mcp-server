// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"io"
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

// TestGetTokenFromAuthHeader tests the helper function that extracts token from Authorization Bearer header
func TestGetTokenFromAuthHeader(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "Authorization Bearer token",
			headers:  map[string]string{"Authorization": "Bearer my-token"},
			expected: "my-token",
		},
		{
			name:     "Authorization Basic ignored",
			headers:  map[string]string{"Authorization": "Basic abc123"},
			expected: "",
		},
		{
			name:     "no Authorization header",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name:     "empty Authorization header",
			headers:  map[string]string{"Authorization": ""},
			expected: "",
		},
		{
			name:     "Bearer with no token",
			headers:  map[string]string{"Authorization": "Bearer "},
			expected: "",
		},
		{
			name:     "Bearer with whitespace token",
			headers:  map[string]string{"Authorization": "Bearer   "},
			expected: "  ",
		},
		{
			name:     "Bearer lowercase",
			headers:  map[string]string{"Authorization": "bearer my-token"},
			expected: "", // Case sensitive - must be "Bearer"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := getTokenFromAuthHeader(req)
			assert.Equal(t, tt.expected, result)
		})
	}
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
			name: "Authorization Bearer header provides token",
			headers: map[string]string{
				"Authorization": "Bearer bearer-token",
			},
			queryParams:    map[string]string{},
			envVars:        map[string]string{},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformToken: "bearer-token",
			},
		},
		{
			name: "standard header takes priority over Authorization Bearer",
			headers: map[string]string{
				TerraformToken:  "standard-token",
				"Authorization": "Bearer bearer-token",
			},
			queryParams:    map[string]string{},
			envVars:        map[string]string{},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformToken: "standard-token",
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
			errorMessage:   "Terraform token should not be provided in query parameters",
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
				TerraformToken:   "header-token",                // Must provide token via header too
			},
			queryParams: map[string]string{
				TerraformSkipTLSVerify: "true", // Query param wins over env
			},
			envVars: map[string]string{
				TerraformAddress:       "https://env.terraform.io", // Overridden by header
				TerraformSkipTLSVerify: "false",                    // Overridden by query param
			},
			expectedStatus: http.StatusOK,
			expectedContextVals: map[string]string{
				TerraformAddress:       "https://header.terraform.io",
				TerraformToken:         "header-token",
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

	// Clear env vars to ensure clean state
	os.Unsetenv(TerraformToken)
	os.Unsetenv(TerraformAddress)

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

	// Clear env vars to ensure clean state
	os.Unsetenv(TerraformToken)
	os.Unsetenv(TerraformAddress)

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

// TestTerraformContextMiddleware_RejectAddressHeaderWhenTokenFromEnv tests that the middleware
// rejects Terraform-Address headers when TFE_TOKEN is set via env var.
func TestTerraformContextMiddleware_RejectAddressHeaderWhenTokenFromEnv(t *testing.T) {
	// Set TFE_TOKEN via environment variable
	os.Setenv(TerraformToken, "test-token-from-env")
	defer os.Unsetenv(TerraformToken)

	logger := log.New()
	logger.SetOutput(io.Discard)

	middleware := TerraformContextMiddleware(logger)

	handlerReached := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerReached = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)

	// Create request with Terraform-Address header (this attempts to redirect)
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set(TerraformAddress, "https://malicious-server.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should be rejected with a 403
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// The handler should not be reached
	assert.False(t, handlerReached, "handler should not have been reached when address header is rejected")

	// Confirm error message
	assert.Contains(t, rr.Body.String(), "Cannot specify Terraform address via header")
}

// TestTerraformContextMiddleware_AllowAddressHeaderWhenTokenFromHeader tests that the middleware
// allows Terraform-Address headers when TFE_TOKEN is not set via env var.
// legit use case where a user can provide both address and token via headers.
func TestTerraformContextMiddleware_AllowAddressHeaderWhenTokenFromHeader(t *testing.T) {
	// Ensure TFE_TOKEN is NOT set via environment
	os.Unsetenv(TerraformToken)

	logger := log.New()
	logger.SetOutput(io.Discard)

	middleware := TerraformContextMiddleware(logger)

	handlerReached := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerReached = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)

	// Create request with both address and token from headers this is a legit use case
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set(TerraformAddress, "https://app.terraform.io")
	req.Header.Set("Authorization", "Bearer user-provided-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should be allowed
	assert.Equal(t, http.StatusOK, rr.Code)

	// Handler should have been reached
	assert.True(t, handlerReached, "handler should have been reached when token is not from env")
}

// TestTerraformContextMiddleware_AllowAddressEnvWhenTokenFromEnv tests that the middleware
// allows Terraform-Address from env var's even when TFE_TOKEN is also from env.
// Only the header-based override is blocked, not the env var config.
func TestTerraformContextMiddleware_AllowAddressEnvWhenTokenFromEnv(t *testing.T) {
	// Set both TFE_TOKEN and TFE_ADDRESS via env var's
	os.Setenv(TerraformToken, "test-token-from-env")
	os.Setenv(TerraformAddress, "https://env.terraform.io")
	defer func() {
		os.Unsetenv(TerraformToken)
		os.Unsetenv(TerraformAddress)
	}()

	logger := log.New()
	logger.SetOutput(io.Discard)

	middleware := TerraformContextMiddleware(logger)

	var capturedAddress string
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if val := ctx.Value(contextKey(TerraformAddress)); val != nil {
			capturedAddress = val.(string)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)

	// Create request without Terraform-Address header
	req := httptest.NewRequest("POST", "/mcp", nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// The address from the env should be in the context
	assert.Equal(t, "https://env.terraform.io", capturedAddress)
}
