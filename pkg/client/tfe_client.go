// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

const (
	TerraformAddress        = "TFE_ADDRESS"
	TerraformToken          = "TFE_TOKEN"
	TerraformSkipTLSVerify  = "TFE_SKIP_VERIFY"
	DefaultTerraformAddress = "https://app.terraform.io"
)

var activeTfeClients sync.Map

// NewTfeClient creates a new TFE client for the given session
func NewTfeClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, logger *log.Logger) (*tfe.Client, error) {
	if terraformToken == "" {
		logger.Warn("No Terraform token provided, TFE client will not be available")
		return nil, utils.LogAndReturnError(logger, "required input: no Terraform token provided", nil)
	}

	config := &tfe.Config{
		Address:           terraformAddress,
		Token:             terraformToken,
		RetryServerErrors: true,
	}

	config.HTTPClient = createHTTPClient(terraformSkipTLSVerify, logger)

	client, err := tfe.NewClient(config)
	if err != nil {
		logger.Warnf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, utils.LogAndReturnError(logger, "creating TFE client", err)
	}

	activeTfeClients.Store(sessionId, client)
	logger.WithField("session_id", sessionId).Info("Created TFE client")
	return client, nil
}

// GetTfeClient retrieves the TFE client for the given session
func GetTfeClient(sessionId string) *tfe.Client {
	if value, ok := activeTfeClients.Load(sessionId); ok {
		return value.(*tfe.Client)
	}
	return nil
}

// DeleteTfeClient removes the TFE client for the given session
func DeleteTfeClient(sessionId string) {
	activeTfeClients.Delete(sessionId)
}

// GetTfeClientFromContext extracts TFE client from the MCP context
func GetTfeClientFromContext(ctx context.Context, logger *log.Logger) (*tfe.Client, error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	// Try to get existing client
	client := GetTfeClient(session.SessionID())
	if client != nil {
		return client, nil
	}

	logger.Warnf("TFE client not found, creating a new one")
	return CreateTfeClientForSession(ctx, session, logger)
}

// CreateTfeClientForSession creates only a TFE client for the session
func CreateTfeClientForSession(ctx context.Context, session server.ClientSession, logger *log.Logger) (*tfe.Client, error) {
	terraformAddress, ok := ctx.Value(contextKey(TerraformAddress)).(string)
	if !ok || terraformAddress == "" {
		terraformAddress = utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	}

	terraformToken, ok := ctx.Value(contextKey(TerraformToken)).(string)
	if !ok || terraformToken == "" {
		terraformToken = utils.GetEnv(TerraformToken, "")
	}

	client, err := NewTfeClient(session.SessionID(), terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken, logger)
	return client, err
}
