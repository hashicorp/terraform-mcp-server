// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
