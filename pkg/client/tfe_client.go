// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

const (
	TerraformAddress        = "TFE_ADDRESS"
	TerraformToken          = "TFE_TOKEN"
	TerraformSkipTLSVerify  = "TFE_SKIP_TLS_VERIFY"
	DefaultTerraformAddress = "https://app.terraform.io"
	ForwardClientIP         = "MCP_FORWARD_CLIENT_IP"
	ClientIPKey             = "CLIENT_IP"
)

var activeTfeClients sync.Map

type cachedTfeClient struct {
	client  *tfe.Client
	token   [32]byte // Store the hash of the token instead of raw value
	address string
}

// NewTfeClient creates a new TFE client for the given session
func NewTfeClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, clientIP string, logger *log.Logger) (*tfe.Client, error) {
	client, err := newTfeClient(terraformAddress, terraformSkipTLSVerify, terraformToken, clientIP, logger)
	if err != nil {
		return nil, err
	}
	// Store the token and address along with the client per session ID
	activeTfeClients.Store(sessionId, cachedTfeClient{
		client:  client,
		token:   sha256.Sum256([]byte(terraformToken)),
		address: terraformAddress,
	})
	logger.Info("Created TFE client")
	return client, nil
}

// NewTfeClientForToken creates a TFE client without storing it in session state.
func NewTfeClientForToken(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, clientIP string, logger *log.Logger) (*tfe.Client, error) {
	return newTfeClient(terraformAddress, terraformSkipTLSVerify, terraformToken, clientIP, logger)
}

func newTfeClient(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, clientIP string, logger *log.Logger) (*tfe.Client, error) {
	if terraformToken == "" {
		logger.Warn("No Terraform token provided, TFE client will not be available")
		return nil, utils.LogAndReturnError(logger, "required input: no Terraform token provided", nil)
	}

	config := &tfe.Config{
		Address:           terraformAddress,
		Token:             terraformToken,
		RetryServerErrors: true,
		Headers:           make(http.Header),
	}

	config.Headers.Set("User-Agent", fmt.Sprintf("terraform-mcp-server/%s", version.GetHumanVersion()))
	if clientIP != "" {
		config.Headers.Set("X-Forwarded-For", clientIP)
	}
	config.HTTPClient = createHTTPClient(terraformSkipTLSVerify, logger)

	client, err := tfe.NewClient(config)
	if err != nil {
		logger.Warnf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, utils.LogAndReturnError(logger, "creating TFE client", err)
	}

	return client, nil
}

// GetTfeClient retrieves the TFE client for the given session
func GetTfeClient(sessionId string) *tfe.Client {
	if value, ok := activeTfeClients.Load(sessionId); ok {
		return value.(cachedTfeClient).client
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

	// Try to get token and address from the current request
	currentAddress := ctx.Value(contextKey(TerraformAddress)).(string)
	if currentAddress == "" {
		currentAddress = utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	}
	currentToken := ctx.Value(contextKey(TerraformToken)).(string)
	if currentToken == "" {
		currentToken = utils.GetEnv(TerraformToken, "")
	}
	// Check if the cached session ID's token+address match the current token+address
	if value, ok := activeTfeClients.Load(session.SessionID()); ok {
		cachedClient := value.(cachedTfeClient)
		currentTokenHash := sha256.Sum256([]byte(currentToken))
		if cachedClient.token == currentTokenHash && cachedClient.address == currentAddress {
			return cachedClient.client, nil
		}
		// Current request token and address not found in cache. Delete the session ID from the sync map.
		activeTfeClients.Delete(session.SessionID())
	}
	logger.Warnf("TFE client not found, creating a new one")
	return CreateTfeClientForSession(ctx, session, logger)
}

// CreateTfeClientForSession creates only a TFE client for the session
func CreateTfeClientForSession(ctx context.Context, session server.ClientSession, logger *log.Logger) (*tfe.Client, error) {
	var err error
	terraformAddress, ok := ctx.Value(contextKey(TerraformAddress)).(string)
	if !ok || terraformAddress == "" {
		terraformAddress = utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	}

	terraformToken, ok := ctx.Value(contextKey(TerraformToken)).(string)
	if !ok || terraformToken == "" {
		terraformToken = utils.GetEnv(TerraformToken, "")
	}
	if terraformToken == "" {
		terraformToken, err = ReadCredentialsFile(extractHostname(terraformAddress), logger)
		if err != nil {
			return nil, err
		}
		logger.Info("Read TFE_TOKEN from credentials.tfrc.json")
	}

	// Get client IP from context for X-Forwarded-For header
	clientIP, _ := ctx.Value(contextKey(ClientIPKey)).(string)
	client, err := NewTfeClient(session.SessionID(), terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken, clientIP, logger)
	return client, err
}
