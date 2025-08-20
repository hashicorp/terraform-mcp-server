// Copyright (c) HashiCorp, Inc.
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
	client := createHTTPClient(terraformSkipTLSVerify, logger)
	activeHttpClients.Store(sessionId, client)
	logger.WithField("session_id", sessionId).Info("Created HTTP client")
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

	// Try to get existing client
	client := GetHttpClient(session.SessionID())
	if client != nil {
		return client, nil
	}

	logger.Warnf("HTTP client not found, creating a new one")
	return CreateHttpClientForSession(ctx, session, logger), nil
}

// CreateHttpClientForSession creates only an HTTP client for the session
func CreateHttpClientForSession(ctx context.Context, session server.ClientSession, logger *log.Logger) *http.Client {
	return NewHttpClient(session.SessionID(), parseTerraformSkipTLSVerify(ctx), logger)
}
