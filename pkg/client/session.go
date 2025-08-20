// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// contextKey is a type alias to avoid lint warnings while maintaining compatibility
type contextKey string

// NewSessionHandler initializes clients for the session
func NewSessionHandler(ctx context.Context, session server.ClientSession, logger *log.Logger) {
	// Create both TFE and HTTP clients for the session
	tfeClient, err := CreateTfeClientForSession(ctx, session, logger)
	if err != nil {
		logger.WithError(err).Error("NewSessionHandler failed to create TFE client")
	}

	CreateHttpClientForSession(ctx, session, logger)

	// Check if the session has a valid TFE client and register with dynamic tool registry
	if tfeClient != nil {
		// Import the tools package to access the registry
		// We need to avoid circular imports, so we'll use a callback approach
		if registryCallback := getToolRegistryCallback(); registryCallback != nil {
			registryCallback.RegisterSessionWithTFE(session.SessionID())
		}
		logger.Info("Session has valid TFE client - registered with tool registry")
	} else {
		logger.Warn("Session has no valid TFE client - TFE tools will not be available")
	}
}

// EndSessionHandler cleans up clients when the session ends
func EndSessionHandler(_ context.Context, session server.ClientSession, logger *log.Logger) {
	// Unregister from tool registry if it was registered
	if registryCallback := getToolRegistryCallback(); registryCallback != nil {
		registryCallback.UnregisterSessionWithTFE(session.SessionID())
	}

	DeleteTfeClient(session.SessionID())
	DeleteHttpClient(session.SessionID())
	logger.WithField("session_id", session.SessionID()).Info("Cleaned up clients for session")
}

// ToolRegistryCallback defines the interface for interacting with the tool registry
type ToolRegistryCallback interface {
	RegisterSessionWithTFE(sessionID string)
	UnregisterSessionWithTFE(sessionID string)
}

var toolRegistryCallback ToolRegistryCallback

// SetToolRegistryCallback sets the callback for tool registry operations
func SetToolRegistryCallback(callback ToolRegistryCallback) {
	toolRegistryCallback = callback
}

// getToolRegistryCallback returns the current tool registry callback
func getToolRegistryCallback() ToolRegistryCallback {
	return toolRegistryCallback
}
