// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins []string
	Mode           string // "strict", "development", "disabled"
}

// LoadCORSConfigFromEnv loads CORS configuration from environment variables
func LoadCORSConfigFromEnv() CORSConfig {
	originsStr := os.Getenv("MCP_ALLOWED_ORIGINS")
	mode := os.Getenv("MCP_CORS_MODE")

	// Default to strict mode if not specified
	if mode == "" {
		mode = "strict"
	}

	var origins []string
	if originsStr != "" {
		origins = strings.Split(originsStr, ",")
		// Trim spaces
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}

	return CORSConfig{
		AllowedOrigins: origins,
		Mode:           mode,
	}
}

// isOriginAllowed checks if the given origin is allowed based on the configuration
func isOriginAllowed(origin string, allowedOrigins []string, mode string) bool {
	// If mode is disabled, allow all origins
	if mode == "disabled" {
		return true
	}

	// Check if origin is in the allowed list
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}

	// In development mode, also allow localhost origins
	if mode == "development" {
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "https://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "https://127.0.0.1:") ||
			strings.HasPrefix(origin, "http://[::1]:") ||
			strings.HasPrefix(origin, "https://[::1]:") {
			return true
		}
	}

	return false
}

// securityHandler wraps the StreamableHTTP handler with origin validation
type securityHandler struct {
	handler        http.Handler
	allowedOrigins []string
	corsMode       string
	logger         *log.Logger
}

// ServeHTTP implements the http.Handler interface
func (h *securityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate Origin header
	origin := r.Header.Get("Origin")
	if origin != "" {
		if !isOriginAllowed(origin, h.allowedOrigins, h.corsMode) {
			h.logger.Warnf("Rejected request from unauthorized origin: %s (CORS mode: %s)", origin, h.corsMode)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Log allowed origins at debug level to avoid too much noise in production
		h.logger.Debugf("Allowed request from origin: %s", origin)

		// If we have a valid origin, add CORS headers
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id")
	}

	// Handle OPTIONS requests for CORS preflight
	if r.Method == http.MethodOptions {
		h.logger.Debugf("Handling OPTIONS preflight request from origin: %s", origin)
		w.WriteHeader(http.StatusOK)
		return
	}

	// If origin is valid or not present, delegate to the wrapped handler
	h.handler.ServeHTTP(w, r)
}

// NewSecurityHandler creates a new security handler
func NewSecurityHandler(handler http.Handler, allowedOrigins []string, corsMode string, logger *log.Logger) http.Handler {
	return &securityHandler{
		handler:        handler,
		allowedOrigins: allowedOrigins,
		corsMode:       corsMode,
		logger:         logger,
	}
}
