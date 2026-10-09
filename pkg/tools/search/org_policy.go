// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

var errQueryRunOwnerUnresolved = errors.New("query run source organization could not be resolved")

// authorizeQueryRunOrganization enforces the MCP organization allowlist, if one is configured,
// against the organization that backend-resolves as owning the query run's source workspace.
// It returns client.ErrOrganizationNotAllowed for a disallowed owner and fails closed when the
// owner cannot be established.
func authorizeQueryRunOrganization(ctx context.Context, tfeClient *tfe.Client, queryRunID string) error {
	if !client.OrganizationAllowlistConfigured(ctx) {
		return nil
	}

	queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
	if err != nil {
		return err
	}
	if queryRun == nil || queryRun.Workspace == nil || queryRun.Workspace.ID == "" {
		return fmt.Errorf("%w: query run has no source workspace", errQueryRunOwnerUnresolved)
	}

	workspace, err := tfeClient.Workspaces.ReadByID(ctx, queryRun.Workspace.ID)
	if err != nil {
		return err
	}
	if workspace == nil || workspace.Organization == nil {
		return fmt.Errorf("%w: workspace %q has no organization", errQueryRunOwnerUnresolved, queryRun.Workspace.ID)
	}

	// go-tfe maps the organization's JSON:API primary ID (its name) to Name.
	organizationName := workspace.Organization.Name
	if organizationName == "" {
		return fmt.Errorf("%w: workspace %q organization has no name", errQueryRunOwnerUnresolved, queryRun.Workspace.ID)
	}
	return client.AuthorizeOrganization(ctx, organizationName)
}

// queryRunDenied returns the tool error for a query run owned by a disallowed organization.
// The organization name is deliberately omitted from the message.
func queryRunDenied(logger *log.Logger, toolName, queryRunID string, err error) (*mcp.CallToolResult, bool) {
	if !errors.Is(err, client.ErrOrganizationNotAllowed) {
		return nil, false
	}
	result, _ := toolErrorf(logger, toolName, "query run %q belongs to an organization that is not allowed by this server", queryRunID)
	return result, true
}
