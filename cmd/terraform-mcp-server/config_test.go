// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		flagValue   string
		expected    log.Level
		description string
	}{
		{
			name:        "env var takes precedence",
			envValue:    "debug",
			flagValue:   "error",
			expected:    log.DebugLevel,
			description: "LOG_LEVEL env var should override --log-level flag",
		},
		{
			name:        "flag used when env not set",
			envValue:    "",
			flagValue:   "warn",
			expected:    log.WarnLevel,
			description: "--log-level flag should be used when LOG_LEVEL is not set",
		},
		{
			name:        "default when neither set",
			envValue:    "",
			flagValue:   "",
			expected:    log.InfoLevel,
			description: "should default to info level when neither env nor flag is set",
		},
		{
			name:        "invalid env falls back to default",
			envValue:    "invalid",
			flagValue:   "",
			expected:    log.InfoLevel,
			description: "invalid LOG_LEVEL should fall back to default info level",
		},
		{
			name:        "invalid flag falls back to default",
			envValue:    "",
			flagValue:   "invalid",
			expected:    log.InfoLevel,
			description: "invalid --log-level should fall back to default info level",
		},
		{
			name:        "env overrides even with invalid flag",
			envValue:    "error",
			flagValue:   "invalid",
			expected:    log.ErrorLevel,
			description: "valid LOG_LEVEL should be used even if flag is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env var
			originalEnv := os.Getenv("LOG_LEVEL")
			defer func() {
				if originalEnv != "" {
					os.Setenv("LOG_LEVEL", originalEnv)
				} else {
					os.Unsetenv("LOG_LEVEL")
				}
			}()

			// Set up test environment
			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			// Create a test command with the flag
			cmd := &cobra.Command{}
			cmd.Flags().String("log-level", tt.flagValue, "test flag")

			// Test the function
			level := getLogLevel(cmd)
			if level != tt.expected {
				t.Errorf("%s: expected level %v, got %v", tt.description, tt.expected, level)
			}
		})
	}
}

func TestGetLogLevelWithNilCommand(t *testing.T) {
	// Save and restore original env var
	originalEnv := os.Getenv("LOG_LEVEL")
	defer func() {
		if originalEnv != "" {
			os.Setenv("LOG_LEVEL", originalEnv)
		} else {
			os.Unsetenv("LOG_LEVEL")
		}
	}()

	// Test with nil command and no env var
	os.Unsetenv("LOG_LEVEL")
	level := getLogLevel(nil)
	if level != log.InfoLevel {
		t.Errorf("expected default info level with nil command, got %v", level)
	}

	// Test with nil command but env var set
	os.Setenv("LOG_LEVEL", "debug")
	level = getLogLevel(nil)
	if level != log.DebugLevel {
		t.Errorf("expected debug level from env var with nil command, got %v", level)
	}
}

func TestInitLoggerWithLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    log.Level
		expected log.Level
	}{
		{"trace level", log.TraceLevel, log.TraceLevel},
		{"debug level", log.DebugLevel, log.DebugLevel},
		{"info level", log.InfoLevel, log.InfoLevel},
		{"warn level", log.WarnLevel, log.WarnLevel},
		{"error level", log.ErrorLevel, log.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with no log file (stdout)
			logger, err := initLogger("", tt.level, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if logger.GetLevel() != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, logger.GetLevel())
			}
		})
	}
}

func TestInitLoggerWithFormat(t *testing.T) {
	tests := []struct {
		name              string
		logFormat         string
		expectedLogFormat log.Formatter
	}{
		{"empty format", "", &log.TextFormatter{}},
		{"text format", "text", &log.TextFormatter{}},
		{"json format", "json", &log.JSONFormatter{}},
		{"TEXT format", "TEXT", &log.TextFormatter{}},
		{"JSON format", "JSON", &log.JSONFormatter{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with no log file (stdout)
			logger, err := initLogger("", log.InfoLevel, tt.logFormat)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tt.expectedLogFormat.(type) {
			case *log.JSONFormatter:
				if _, ok := logger.Formatter.(*log.JSONFormatter); !ok {
					t.Errorf("expected JSONFormatter, got %T", logger.Formatter)
				}
			case *log.TextFormatter:
				if _, ok := logger.Formatter.(*log.TextFormatter); !ok {
					t.Errorf("expected TextFormatter, got %T", logger.Formatter)
				}
			}
		})
	}
}

func TestGetLogFormat(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		flagValue   string
		expected    string
		description string
	}{
		{
			name:        "env var takes precedence",
			envValue:    "json",
			flagValue:   "text",
			expected:    "json",
			description: "LOG_FORMAT env var should override --log-format flag",
		},
		{
			name:        "flag used when env not set",
			envValue:    "",
			flagValue:   "json",
			expected:    "json",
			description: "--log-format flag should be used when LOG_FORMAT is not set",
		},
		{
			name:        "default when neither set",
			envValue:    "",
			flagValue:   "",
			expected:    "text",
			description: "should default to text format when neither env nor flag is set",
		},
		{
			name:        "invalid env falls back to default",
			envValue:    "invalid",
			flagValue:   "",
			expected:    "text",
			description: "invalid LOG_FORMAT should fall back to default text format",
		},
		{
			name:        "invalid flag falls back to default",
			envValue:    "",
			flagValue:   "invalid",
			expected:    "text",
			description: "invalid --log-format should fall back to default text format",
		},
		{
			name:        "env overrides even with invalid flag",
			envValue:    "json",
			flagValue:   "invalid",
			expected:    "json",
			description: "valid LOG_FORMAT should be used even if flag is invalid",
		},
		{
			name:        "case insensitive - uppercase JSON",
			envValue:    "JSON",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should be case insensitive (JSON -> json)",
		},
		{
			name:        "case insensitive - uppercase TEXT",
			envValue:    "TEXT",
			flagValue:   "",
			expected:    "text",
			description: "LOG_FORMAT should be case insensitive (TEXT -> text)",
		},
		{
			name:        "case insensitive - mixed case Json",
			envValue:    "Json",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should be case insensitive (Json -> json)",
		},
		{
			name:        "whitespace trimmed from env",
			envValue:    "  json  ",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should trim whitespace from env value",
		},
		{
			name:        "whitespace trimmed from flag",
			envValue:    "",
			flagValue:   "  text  ",
			expected:    "text",
			description: "--log-format should trim whitespace from flag value",
		},
		{
			name:        "text format explicitly set via env",
			envValue:    "text",
			flagValue:   "",
			expected:    "text",
			description: "text format should be explicitly settable via LOG_FORMAT",
		},
		{
			name:        "text format explicitly set via flag",
			envValue:    "",
			flagValue:   "text",
			expected:    "text",
			description: "text format should be explicitly settable via --log-format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env var
			originalEnv := os.Getenv("LOG_FORMAT")
			defer func() {
				if originalEnv != "" {
					os.Setenv("LOG_FORMAT", originalEnv)
				} else {
					os.Unsetenv("LOG_FORMAT")
				}
			}()

			// Set up test environment
			if tt.envValue != "" {
				os.Setenv("LOG_FORMAT", tt.envValue)
			} else {
				os.Unsetenv("LOG_FORMAT")
			}

			// Create a test command with the flag
			cmd := &cobra.Command{}
			cmd.Flags().String("log-format", tt.flagValue, "test flag")

			// Test the function
			format := getLogFormat(cmd)
			if format != tt.expected {
				t.Errorf("%s: expected format %q, got %q", tt.description, tt.expected, format)
			}
		})
	}
}

func TestGetLogFormatWithNilCommand(t *testing.T) {
	// Save and restore original env var
	originalEnv := os.Getenv("LOG_FORMAT")
	defer func() {
		if originalEnv != "" {
			os.Setenv("LOG_FORMAT", originalEnv)
		} else {
			os.Unsetenv("LOG_FORMAT")
		}
	}()

	// Test with nil command and no env var
	os.Unsetenv("LOG_FORMAT")
	format := getLogFormat(nil)
	if format != "text" {
		t.Errorf("expected default text format with nil command, got %q", format)
	}

	// Test with nil command but env var set to json
	os.Setenv("LOG_FORMAT", "json")
	format = getLogFormat(nil)
	if format != "json" {
		t.Errorf("expected json format from env var with nil command, got %q", format)
	}

	// Test with nil command but env var set to text
	os.Setenv("LOG_FORMAT", "text")
	format = getLogFormat(nil)
	if format != "text" {
		t.Errorf("expected text format from env var with nil command, got %q", format)
	}

	// Test with nil command and invalid env var
	os.Setenv("LOG_FORMAT", "invalid")
	format = getLogFormat(nil)
	if format != "text" {
		t.Errorf("expected default text format with invalid env var and nil command, got %q", format)
	}
}
