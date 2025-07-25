// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHTTPHost(t *testing.T) {
	// Save original env var to restore later
	origHost := os.Getenv("TRANSPORT_HOST")
	defer func() {
		os.Setenv("TRANSPORT_HOST", origHost)
	}()

	// Test case: When TRANSPORT_HOST is not set, default value should be used
	os.Unsetenv("TRANSPORT_HOST")
	host := getHTTPHost()
	assert.Equal(t, "127.0.0.1", host, "Default host should be 127.0.0.1 when TRANSPORT_HOST is not set")

	// Test case: When TRANSPORT_HOST is set, its value should be used
	os.Setenv("TRANSPORT_HOST", "0.0.0.0")
	host = getHTTPHost()
	assert.Equal(t, "0.0.0.0", host, "Host should be the value of TRANSPORT_HOST when it is set")

	// Test case: Custom host value
	os.Setenv("TRANSPORT_HOST", "192.168.1.100")
	host = getHTTPHost()
	assert.Equal(t, "192.168.1.100", host, "Host should be the custom value set in TRANSPORT_HOST")
}

func TestGetEndpointPath(t *testing.T) {
	// Save original env var to restore later
	origPath := os.Getenv("MCP_ENDPOINT")
	defer func() {
		os.Setenv("MCP_ENDPOINT", origPath)
	}()

	// Test case: When MCP_ENDPOINT is not set, default value should be used
	os.Unsetenv("MCP_ENDPOINT")
	path := getEndpointPath(nil)
	assert.Equal(t, "/mcp", path, "Default endpoint path should be /mcp when MCP_ENDPOINT is not set")

	// Test case: When MCP_ENDPOINT is set, its value should be used
	os.Setenv("MCP_ENDPOINT", "/terraform")
	path = getEndpointPath(nil)
	assert.Equal(t, "/terraform", path, "Endpoint path should be the value of MCP_ENDPOINT when it is set")

	// Test case: Custom endpoint path value
	os.Setenv("MCP_ENDPOINT", "/api/v1/terraform-mcp")
	path = getEndpointPath(nil)
	assert.Equal(t, "/api/v1/terraform-mcp", path, "Endpoint path should be the custom value set in MCP_ENDPOINT")

}

func TestGetHTTPPort(t *testing.T) {
	// Save original env var to restore later
	origPort := os.Getenv("TRANSPORT_PORT")
	defer func() {
		os.Setenv("TRANSPORT_PORT", origPort)
	}()

	// Test case: When TRANSPORT_PORT is not set, default value should be used
	os.Unsetenv("TRANSPORT_PORT")
	port := getHTTPPort()
	assert.Equal(t, "8080", port, "Default port should be 8080 when TRANSPORT_PORT is not set")

	// Test case: When TRANSPORT_PORT is set, its value should be used
	os.Setenv("TRANSPORT_PORT", "9090")
	port = getHTTPPort()
	assert.Equal(t, "9090", port, "Port should be the value of TRANSPORT_PORT when it is set")
}

func TestShouldUseStreamableHTTPMode(t *testing.T) {
	// Save original env vars to restore later
	origMode := os.Getenv("TRANSPORT_MODE")
	origPort := os.Getenv("TRANSPORT_PORT")
	origHost := os.Getenv("TRANSPORT_HOST")
	origEndpointPath := os.Getenv("MCP_ENDPOINT")
	defer func() {
		os.Setenv("TRANSPORT_MODE", origMode)
		os.Setenv("TRANSPORT_PORT", origPort)
		os.Setenv("TRANSPORT_HOST", origHost)
		os.Setenv("MCP_ENDPOINT", origEndpointPath)
	}()

	// Test case: When no relevant env vars are set, HTTP mode should not be used
	os.Unsetenv("TRANSPORT_MODE")
	os.Unsetenv("TRANSPORT_PORT")
	os.Unsetenv("TRANSPORT_HOST")
	os.Unsetenv("MCP_ENDPOINT")
	assert.False(t, shouldUseStreamableHTTPMode(), "HTTP mode should not be used when no relevant env vars are set")

	// Test case: When TRANSPORT_MODE is set to "http", HTTP mode should be used (backward compatibility)
	os.Setenv("TRANSPORT_MODE", "http")
	assert.True(t, shouldUseStreamableHTTPMode(), "HTTP mode should be used when TRANSPORT_MODE is set to 'http'")
	os.Unsetenv("TRANSPORT_MODE")

	// Test case: When TRANSPORT_MODE is set to "streamable-http", HTTP mode should be used
	os.Setenv("TRANSPORT_MODE", "streamable-http")
	assert.True(t, shouldUseStreamableHTTPMode(), "HTTP mode should be used when TRANSPORT_MODE is set to 'streamable-http'")
	os.Unsetenv("TRANSPORT_MODE")

	// Test case: When TRANSPORT_PORT is set, HTTP mode should be used
	os.Setenv("TRANSPORT_PORT", "9090")
	assert.True(t, shouldUseStreamableHTTPMode(), "HTTP mode should be used when TRANSPORT_PORT is set")
	os.Unsetenv("TRANSPORT_PORT")

	// Test case: When TRANSPORT_HOST is set, HTTP mode should be used
	os.Setenv("TRANSPORT_HOST", "0.0.0.0")
	assert.True(t, shouldUseStreamableHTTPMode(), "HTTP mode should be used when TRANSPORT_HOST is set")
	os.Unsetenv("TRANSPORT_HOST")

	// Test case: When MCP_ENDPOINT is set, HTTP mode should be used
	os.Setenv("MCP_ENDPOINT", "/mcp")
	assert.True(t, shouldUseStreamableHTTPMode(), "HTTP mode should be used when MCP_ENDPOINT is set")

}
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
