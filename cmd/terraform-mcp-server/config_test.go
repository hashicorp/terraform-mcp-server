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

func TestShouldUseHTTPMode(t *testing.T) {
	// Save original env vars to restore later
	origMode := os.Getenv("TRANSPORT_MODE")
	origPort := os.Getenv("TRANSPORT_PORT")
	origHost := os.Getenv("TRANSPORT_HOST")
	defer func() {
		os.Setenv("TRANSPORT_MODE", origMode)
		os.Setenv("TRANSPORT_PORT", origPort)
		os.Setenv("TRANSPORT_HOST", origHost)
	}()

	// Test case: When no relevant env vars are set, HTTP mode should not be used
	os.Unsetenv("TRANSPORT_MODE")
	os.Unsetenv("TRANSPORT_PORT")
	os.Unsetenv("TRANSPORT_HOST")
	assert.False(t, shouldUseHTTPMode(), "HTTP mode should not be used when no relevant env vars are set")

	// Test case: When TRANSPORT_MODE is set to "http", HTTP mode should be used
	os.Setenv("TRANSPORT_MODE", "http")
	assert.True(t, shouldUseHTTPMode(), "HTTP mode should be used when TRANSPORT_MODE is set to 'http'")
	os.Unsetenv("TRANSPORT_MODE")

	// Test case: When TRANSPORT_PORT is set, HTTP mode should be used
	os.Setenv("TRANSPORT_PORT", "9090")
	assert.True(t, shouldUseHTTPMode(), "HTTP mode should be used when TRANSPORT_PORT is set")
	os.Unsetenv("TRANSPORT_PORT")

	// Test case: When TRANSPORT_HOST is set, HTTP mode should be used
	os.Setenv("TRANSPORT_HOST", "0.0.0.0")
	assert.True(t, shouldUseHTTPMode(), "HTTP mode should be used when TRANSPORT_HOST is set")
}
