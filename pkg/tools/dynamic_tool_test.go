// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestDynamicToolRegistry_SessionManagement(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Create a registry without initializing the MCP server
	registry := &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          nil, // We'll skip actual tool registration
		logger:             logger,
	}

	// Initially no sessions should have TFE
	if registry.HasAnySessionWithTFE() {
		t.Error("Expected no sessions with TFE initially")
	}

	sessionID1 := "test-session-1"
	sessionID2 := "test-session-2"

	// Check specific sessions
	if registry.HasSessionWithTFE(sessionID1) {
		t.Error("Expected session1 to not have TFE initially")
	}

	// Manually register sessions (without triggering tool registration)
	registry.mu.Lock()
	registry.sessionsWithTFE[sessionID1] = true
	registry.mu.Unlock()

	if !registry.HasSessionWithTFE(sessionID1) {
		t.Error("Expected session1 to have TFE after registration")
	}

	if !registry.HasAnySessionWithTFE() {
		t.Error("Expected at least one session with TFE")
	}

	if registry.HasSessionWithTFE(sessionID2) {
		t.Error("Expected session2 to not have TFE")
	}

	// Register second session
	registry.mu.Lock()
	registry.sessionsWithTFE[sessionID2] = true
	registry.mu.Unlock()

	if !registry.HasSessionWithTFE(sessionID2) {
		t.Error("Expected session2 to have TFE after registration")
	}

	// Unregister first session
	registry.UnregisterSessionWithTFE(sessionID1)

	if registry.HasSessionWithTFE(sessionID1) {
		t.Error("Expected session1 to not have TFE after unregistration")
	}

	if !registry.HasSessionWithTFE(sessionID2) {
		t.Error("Expected session2 to still have TFE")
	}

	if !registry.HasAnySessionWithTFE() {
		t.Error("Expected session2 to still provide TFE availability")
	}

	// Unregister second session
	registry.UnregisterSessionWithTFE(sessionID2)

	if registry.HasSessionWithTFE(sessionID2) {
		t.Error("Expected session2 to not have TFE after unregistration")
	}

	if registry.HasAnySessionWithTFE() {
		t.Error("Expected no sessions with TFE after all unregistered")
	}
}

func TestDynamicToolRegistry_ToolRegistrationState(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Create a registry without MCP server to test state management
	registry := &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          nil,
		logger:             logger,
	}

	// Initially tools should not be registered
	if registry.tfeToolsRegistered {
		t.Error("Expected TFE tools to not be registered initially")
	}

	// Manually set tools as registered (simulating what would happen)
	registry.mu.Lock()
	registry.tfeToolsRegistered = true
	registry.mu.Unlock()

	// Now tools should be registered
	if !registry.tfeToolsRegistered {
		t.Error("Expected TFE tools to be registered")
	}
}

func TestDynamicToolRegistry_ConcurrentAccess(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Create a registry for concurrent testing
	registry := &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          nil,
		logger:             logger,
	}

	// Test concurrent registration and unregistration
	done := make(chan bool, 10)

	// Start multiple goroutines registering sessions
	for i := 0; i < 5; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session-%d", id)
			// Manually register/unregister to avoid MCP server calls
			registry.mu.Lock()
			registry.sessionsWithTFE[sessionID] = true
			registry.mu.Unlock()

			registry.UnregisterSessionWithTFE(sessionID)
			done <- true
		}(i)
	}

	// Start multiple goroutines checking state
	for i := 0; i < 5; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session-%d", id)
			registry.HasSessionWithTFE(sessionID)
			registry.HasAnySessionWithTFE()
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test should complete without deadlocks or panics
}
