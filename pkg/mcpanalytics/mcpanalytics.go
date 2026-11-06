// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

// Package mcpanalytics sends MCP analytics for this server to a PostHog
// project chosen by whoever runs it. Nothing is sent unless
// MCP_POSTHOG_PROJECT_TOKEN is set.
package mcpanalytics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/posthog/posthog-go"
	"github.com/posthog/posthog-go/posthogmcp"
	"github.com/posthog/posthog-go/posthogmcpsdk"
	log "github.com/sirupsen/logrus"
)

const shutdownTimeout = 2 * time.Second

const orgLookupTimeout = 2 * time.Second

// Analytics sends tool calls to PostHog. A nil *Analytics sends nothing, so
// callers need no check of their own.
type Analytics struct {
	client       posthog.Client
	logger       *log.Logger
	orgIDsByName sync.Map
	lookupOrgID  func(ctx context.Context, req *mcp.CallToolRequest, orgName string) (string, error)
}

// Start returns the analytics configured by MCP_POSTHOG_PROJECT_TOKEN and
// MCP_POSTHOG_HOST, or nil when no token is set. Close must run before exit to
// send queued events.
func Start(logger *log.Logger) *Analytics {
	projectToken := os.Getenv("MCP_POSTHOG_PROJECT_TOKEN")
	if projectToken == "" {
		return nil
	}
	client, err := posthog.NewWithConfig(projectToken, posthog.Config{
		Endpoint:        os.Getenv("MCP_POSTHOG_HOST"),
		ShutdownTimeout: shutdownTimeout,
	})
	if err != nil {
		logger.Errorf("MCP analytics are off: %v", err)
		return nil
	}
	logger.Infof("Sending MCP analytics to PostHog")
	return &Analytics{client: client, logger: logger, lookupOrgID: readOrgID}
}

func (a *Analytics) Close() {
	if a == nil {
		return
	}
	if err := a.client.Close(); err != nil {
		a.logger.Errorf("Failed to send queued MCP analytics: %v", err)
	}
}

// Instrument sends srv's tool calls to PostHog. Tool arguments and results
// are never sent, since they can hold customer data.
func (a *Analytics) Instrument(srv *mcp.Server) {
	if a == nil {
		return
	}
	posthogmcpsdk.Instrument(srv, posthogmcp.New(a.client),
		posthogmcpsdk.WithServerInfo("terraform-mcp-server", version.Version),
		posthogmcpsdk.WithCaptureParameters(false),
		posthogmcpsdk.WithCaptureResponses(false),
		// It would add a conversation_id argument to every tool, and a text
		// block to tool results in stateless mode.
		posthogmcpsdk.WithConversationID(false),
		posthogmcpsdk.WithProperties(a.orgProperties),
		posthogmcpsdk.WithErrorHandler(func(_ context.Context, err error) {
			a.logger.Warnf("MCP analytics: %v", err)
		}),
	)
}

func (a *Analytics) orgProperties(ctx context.Context, req *mcp.CallToolRequest, _ *mcp.CallToolResult, _ error) (posthog.Properties, error) {
	var arguments struct {
		OrgName string `json:"terraform_org_name"`
	}
	if req == nil || req.Params == nil || json.Unmarshal(req.Params.Arguments, &arguments) != nil {
		return nil, nil
	}
	orgName := strings.TrimSpace(arguments.OrgName)
	if orgName == "" {
		return nil, nil
	}
	orgID, err := a.orgID(ctx, req, orgName)
	if err != nil {
		return nil, fmt.Errorf("reading organization %q: %w", orgName, err)
	}
	return posthog.Properties{"terraform_org_id": orgID, "terraform_org_name": strings.ToLower(orgName)}, nil
}

func (a *Analytics) orgID(ctx context.Context, req *mcp.CallToolRequest, orgName string) (string, error) {
	if orgID, ok := a.orgIDsByName.Load(strings.ToLower(orgName)); ok {
		return orgID.(string), nil
	}
	ctx, cancel := context.WithTimeout(ctx, orgLookupTimeout)
	defer cancel()
	orgID, err := a.lookupOrgID(ctx, req, orgName)
	if err != nil {
		return "", err
	}
	a.orgIDsByName.Store(strings.ToLower(orgName), orgID)
	return orgID, nil
}

func readOrgID(ctx context.Context, req *mcp.CallToolRequest, orgName string) (string, error) {
	tfeClient, err := officialclient.GetTfeClient(ctx, officialclient.SessionIDFromRequest(req))
	if err != nil {
		return "", err
	}
	org, err := tfeClient.Organizations.Read(ctx, orgName)
	if err != nil {
		return "", err
	}
	return org.ExternalID, nil
}
