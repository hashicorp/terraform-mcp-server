// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
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
	SharedSecretEnv         = "TF_MCP_SHARED_SECRET"
	SharedSecretHeader      = "X-Tf-Mcp-Secret"
)

var activeTfeClients sync.Map

type cachedTfeClient struct {
	client *tfe.Client
	token  [32]byte // Store the hash of the token instead of raw value
}

// NewTfeClient creates a new TFE client for the given session
func NewTfeClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, clientIP string, logger *log.Logger) (*tfe.Client, error) {
	client, err := newTfeClient(terraformAddress, terraformSkipTLSVerify, terraformToken, clientIP, logger)
	if err != nil {
		return nil, err
	}
	// Store the token and address along with the client per session ID
	activeTfeClients.Store(sessionId, cachedTfeClient{
		client: client,
		token:  sha256.Sum256([]byte(terraformToken)),
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
		return nil, errors.New("required input: no Terraform token provided")
	}

	config := buildTFEConfig(terraformAddress, terraformSkipTLSVerify, terraformToken, clientIP, logger)

	client, err := tfe.NewClient(config)
	if err != nil {
		logger.Warnf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, utils.LogAndReturnError(logger, "creating TFE client", err)
	}

	return client, nil
}

// buildTFEConfig assembles the go-tfe client configuration, including the outbound
// headers set on every request (User-Agent, optional X-Forwarded-For, and the
// optional shared secret).
//
// It is split out from newTfeClient so the header logic can be unit-tested: tfe.NewClient
// consumes the config and does not expose the headers back, so the tests assert against
// the *tfe.Config returned here instead.
func buildTFEConfig(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string, clientIP string, logger *log.Logger) *tfe.Config {
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

	// Attach the shared secret (if configured) so the backend (HCP Terraform / TFE)
	// can identify requests from a trusted MCP deployment.
	if secret := utils.GetEnv(SharedSecretEnv, ""); secret != "" {
		config.Headers.Set(SharedSecretHeader, secret)
	}

	// HCP Terraform and Terraform Enterprise may return temporary signed Archivist
	// URLs for logs and artifacts. CreateHTTPClient redacts sensitive path and query
	// components before the retry client writes request, retry, or failure messages.
	config.HTTPClient = CreateHTTPClient(terraformSkipTLSVerify, logger)
	return config
}

// HumanReadableTokenPermissions returns the display names of the permissions enabled for an organization.
func HumanReadableTokenPermissions(permissions *tfe.OrganizationPermissions) []string {
	if permissions == nil {
		return nil
	}

	permissionsByName := []struct {
		name    string
		enabled bool
	}{
		{"Create Teams", permissions.CanCreateTeam},
		{"Create Workspaces", permissions.CanCreateWorkspace},
		{"Create Workspace Migrations", permissions.CanCreateWorkspaceMigration},
		{"Deploy NoCode Modules", permissions.CanDeployNoCodeModules},
		{"Destroy", permissions.CanDestroy},
		{"Manage Auditing", permissions.CanManageAuditing},
		{"Manage NoCodeModules", permissions.CanManageNoCodeModules},
		{"Manage Run Tasks", permissions.CanManageRunTasks},
		{"Traverse", permissions.CanTraverse},
		{"Update", permissions.CanUpdate},
		{"Update API Tokens", permissions.CanUpdateAPIToken},
		{"Update OAuth", permissions.CanUpdateOAuth},
		{"Update Sentinel", permissions.CanUpdateSentinel},
		{"Update HYOK Configuration", permissions.CanUpdateHYOKConfiguration},
		{"View HYOK Feature Information", permissions.CanViewHYOKFeatureInfo},
		{"Enable Stacks", permissions.CanEnableStacks},
		{"Create Projects", permissions.CanCreateProject},
	}

	permissionNames := make([]string, 0, len(permissionsByName))
	for _, permission := range permissionsByName {
		if permission.enabled {
			permissionNames = append(permissionNames, permission.name)
		}
	}

	return permissionNames
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
		return nil, fmt.Errorf("No active session found")
	}
	return GetTfeClientForSession(ctx, session.SessionID(), logger)
}

// GetTfeClientForSession allows both transports (mark3labs' server.ClientSession and the official
// go-sdk's *mcp.ServerSession) supply their own session ID. The actual
// address/token/skip-TLS-verify lookups still happen against ctx, using
// this package's contextKey type - the same type TerraformContextMiddleware
// writes into ctx, so both transports have the same per-request values
func GetTfeClientForSession(ctx context.Context, sessionID string, logger *log.Logger) (*tfe.Client, error) {
	terraformAddress := terraformAddressForRequest(ctx)
	terraformToken, err := terraformTokenForRequest(ctx, terraformAddress, logger)
	if err != nil {
		return nil, err
	}

	// In a stateless mode the server does not assign any session ID to requests. We need to create new TF clients for every request in that case
	if sessionID == "" {
		logger.Info("Session ID is empty. Creating a new TF client")
		clientIP, _ := ctx.Value(contextKey(ClientIPKey)).(string)
		return NewTfeClientForToken(terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken, clientIP, logger)
	}

	// Reuse the session's client as long as the token hasn't changed
	if value, ok := activeTfeClients.Load(sessionID); ok {
		cachedClient := value.(cachedTfeClient)
		if cachedClient.token == sha256.Sum256([]byte(terraformToken)) {
			return cachedClient.client, nil
		}
		// The token changed, so drop the old client and build a new one
		activeTfeClients.Delete(sessionID)
	}
	logger.Warnf("TFE client not found, creating a new one")
	return newSessionTfeClient(ctx, sessionID, terraformAddress, terraformToken, logger)
}

// CreateTfeClientForSession creates only a TFE client for the session
func CreateTfeClientForSession(ctx context.Context, sessionID string, logger *log.Logger) (*tfe.Client, error) {
	terraformAddress := terraformAddressForRequest(ctx)
	terraformToken, err := terraformTokenForRequest(ctx, terraformAddress, logger)
	if err != nil {
		return nil, err
	}
	return newSessionTfeClient(ctx, sessionID, terraformAddress, terraformToken, logger)
}

func newSessionTfeClient(ctx context.Context, sessionID, terraformAddress, terraformToken string, logger *log.Logger) (*tfe.Client, error) {
	// Get client IP from context for X-Forwarded-For header
	clientIP, _ := ctx.Value(contextKey(ClientIPKey)).(string)
	return NewTfeClient(sessionID, terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken, clientIP, logger)
}

// terraformAddressForRequest returns the address TerraformContextMiddleware put
// in ctx, or the TFE_ADDRESS env var, or the default.
func terraformAddressForRequest(ctx context.Context) string {
	if address, _ := ctx.Value(contextKey(TerraformAddress)).(string); address != "" {
		return address
	}
	return utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
}

// terraformTokenForRequest gets the token for the request. It checks ctx first
// (set by TerraformContextMiddleware), then TFE_TOKEN, then
// credentials.tfrc.json from terraform login. Everything uses this so the cache
// check always compares against the same token the client was built with.
//
// No credentials file, or no entry for this host, just means there's no token,
// so it returns "" and building the client fails with "no Terraform token
// provided". A file that's there but can't be read or parsed returns the error
// so it doesn't get hidden behind "no token".
func terraformTokenForRequest(ctx context.Context, terraformAddress string, logger *log.Logger) (string, error) {
	if token, _ := ctx.Value(contextKey(TerraformToken)).(string); token != "" {
		return token, nil
	}
	if token := utils.GetEnv(TerraformToken, ""); token != "" {
		return token, nil
	}
	token, err := ReadCredentialsFile(extractHostname(terraformAddress), logger)
	if err != nil {
		var noCreds *noCredentialsForHostError
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &noCreds) {
			return "", nil
		}
		return "", fmt.Errorf("reading credentials.tfrc.json: %w", err)
	}
	logger.Debug("Using TFE_TOKEN from credentials.tfrc.json")
	return token, nil
}
