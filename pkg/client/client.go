// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

var (
	activeClients sync.Map
)

const (
	TerraformAddress       = "TFE_ADDRESS"
	TerraformToken         = "TFE_TOKEN"
	TerraformSkipTLSVerify = "TFE_SKIP_VERIFY"
)

const DefaultTerraformAddress = "https://app.terraform.io"

type terraformClients struct {
	TfeClient  *tfe.Client
	HttpClient *http.Client
}

// contextKey is a type alias to avoid lint warnings while maintaining compatibility
type contextKey string

// getEnv retrieves the value of an environment variable or returns a fallback value if not set
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// NewTerraformClient creates a new Terraform client for the given session
func NewTerraformClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, logger *log.Logger) *terraformClients {
	// Initialize Terraform client
	config := &tfe.Config{
		Address:           terraformAddress,
		Token:             terraformToken,
		RetryServerErrors: true,
	}

	config.HTTPClient = createHTTPClient(terraformSkipTLSVerify, logger)
	terraformClients := &terraformClients{
		TfeClient:  nil,
		HttpClient: config.HTTPClient,
	}

	client, err := tfe.NewClient(config)
	if err != nil {
		logger.Warnf("Failed to create a Terraform client: %s, %v", sessionId, err)
		return terraformClients
	}

	terraformClients.TfeClient = client
	activeClients.Store(sessionId, terraformClients)
	return terraformClients
}

// GetTerraformClient retrieves the Terraform client for the given session
func GetTerraformClient(sessionId string) *terraformClients {
	if value, ok := activeClients.Load(sessionId); ok {
		return value.(*terraformClients)
	}
	return nil
}

// DeleteTerraformClient removes the Terraform client for the given session
func DeleteTerraformClient(sessionId string) {
	activeClients.Delete(sessionId)
}

// GetTerraformClientFromContext extracts Terraform client from the MCP context
func GetTerraformClientFromContext(ctx context.Context, logger *log.Logger) (*terraformClients, error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	// Log the session ID for debugging
	logger.WithField("session_id", session.SessionID()).Debug("Retrieving Terraform client for session")

	// Try to get existing client
	client := GetTerraformClient(session.SessionID())
	if client != nil {
		return client, nil
	}

	logger.WithField("session_id", session.SessionID()).Warn("Terraform client not found, creating a new one")
	return CreateTerraformClientForSession(ctx, session, logger)
}

func CreateTerraformClientForSession(ctx context.Context, session server.ClientSession, logger *log.Logger) (*terraformClients, error) {
	// Initialize a new Terraform client for this session
	terraformAddress, ok := ctx.Value(contextKey(TerraformAddress)).(string)
	if !ok || terraformAddress == "" {
		terraformAddress = getEnv(TerraformAddress, DefaultTerraformAddress)
	}

	terraformToken, ok := ctx.Value(contextKey(TerraformToken)).(string)
	if !ok || terraformToken == "" {
		terraformToken = getEnv(TerraformToken, "")
		if terraformToken == "" {
			logger.Warn("Terraform token not provided for session")
		}
	}

	terraformSkipTLSVerifyStr, ok := ctx.Value(contextKey(TerraformSkipTLSVerify)).(string)
	terraformSkipTLSVerify := false
	if ok && terraformSkipTLSVerifyStr != "" {
		var err error
		terraformSkipTLSVerify, err = strconv.ParseBool(terraformSkipTLSVerifyStr)
		if err != nil {
			terraformSkipTLSVerify = false
		}
	}

	newClient := NewTerraformClient(session.SessionID(), terraformAddress, terraformSkipTLSVerify, terraformToken, logger)
	logger.WithFields(log.Fields{
		"session_id": session.SessionID(),
	}).Info("Created Terraform client for session")

	return newClient, nil
}

// NewSessionHandler initializes a new Terraform client for the session
func NewSessionHandler(ctx context.Context, session server.ClientSession, logger *log.Logger) {
	_, err := CreateTerraformClientForSession(ctx, session, logger)
	if err != nil {
		logger.WithError(err).Error("NewSessionHandler failed to create Terraform client")
		return
	}
}

// EndSessionHandler cleans up the Terraform client when the session ends
func EndSessionHandler(_ context.Context, session server.ClientSession, logger *log.Logger) {
	DeleteTerraformClient(session.SessionID())
	logger.WithField("session_id", session.SessionID()).Info("Cleaned up Terraform client for session")
}
