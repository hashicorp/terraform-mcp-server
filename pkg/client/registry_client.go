// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

var (
	activeHttpClients sync.Map
)

// NewHttpClient creates a new HTTP client for the given session
func NewHttpClient(sessionId string, terraformSkipTLSVerify bool, logger *log.Logger) *http.Client {
	client := CreateHTTPClient(terraformSkipTLSVerify, logger)
	activeHttpClients.Store(sessionId, client)
	logger.Info("Created HTTP client")
	return client
}

// GetHttpClient retrieves the HTTP client for the given session
func GetHttpClient(sessionId string) *http.Client {
	if value, ok := activeHttpClients.Load(sessionId); ok {
		return value.(*http.Client)
	}
	return nil
}

// DeleteHttpClient removes the HTTP client for the given session
func DeleteHttpClient(sessionId string) {
	activeHttpClients.Delete(sessionId)
}

// GetHttpClientFromContext extracts HTTP client from the MCP context
func GetHttpClientFromContext(ctx context.Context, logger *log.Logger) (*http.Client, error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	return GetHttpClientForSession(ctx, session.SessionID(), logger), nil
}

// GetHttpClientForSession adds the same stateless (empty sessionID) check and builds without caching a fresh client, instead of every
// stateless request sharing one cached entry
func GetHttpClientForSession(ctx context.Context, sessionID string, logger *log.Logger) *http.Client {
	skipTLSVerify := parseTerraformSkipTLSVerify(ctx)

	if sessionID == "" {
		return CreateHTTPClient(skipTLSVerify, logger)
	}

	// Try to get existing client
	if client := GetHttpClient(sessionID); client != nil {
		return client
	}

	logger.Warnf("HTTP client not found, creating a new one")
	return NewHttpClient(sessionID, skipTLSVerify, logger)
}

// CreateHttpClientForSession creates only an HTTP client for the session
func CreateHttpClientForSession(ctx context.Context, sessionID string, logger *log.Logger) *http.Client {
	return NewHttpClient(sessionID, parseTerraformSkipTLSVerify(ctx), logger)
}
