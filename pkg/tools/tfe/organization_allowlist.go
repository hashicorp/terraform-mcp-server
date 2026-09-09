// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"net/url"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

func checkOrganizationAllowed(ctx context.Context, logger *log.Logger, resource, resourceID string, org *tfe.Organization) (*mcp.CallToolResult, error) {
	orgName := ""
	if org != nil {
		orgName = org.Name
	}
	if client.OrganizationAllowed(ctx, orgName) {
		return nil, nil
	}
	return ToolErrorf(logger, "%s %q belongs to organization %q, which is not allowed by this server", resource, resourceID, orgName)
}

func checkVariableSetOrganizationAllowed(ctx context.Context, tfeClient *tfe.Client, logger *log.Logger, varSetID string) (*mcp.CallToolResult, error) {
	varSet, err := tfeClient.VariableSets.Read(ctx, varSetID, nil)
	if err != nil {
		return ToolErrorf(logger, "variable set not found: %s", varSetID)
	}
	return checkOrganizationAllowed(ctx, logger, "variable set", varSetID, varSet.Organization)
}

// go-tfe's Team type does not expose the organization relationship, so read it from the raw API.
func teamOrganization(ctx context.Context, tfeClient *tfe.Client, teamID string) (*tfe.Organization, error) {
	req, err := tfeClient.NewRequest("GET", "teams/"+url.PathEscape(teamID), nil)
	if err != nil {
		return nil, err
	}
	team := &struct {
		ID           string            `jsonapi:"primary,teams"`
		Organization *tfe.Organization `jsonapi:"relation,organization"`
	}{}
	if err := req.Do(ctx, team); err != nil {
		return nil, err
	}
	return team.Organization, nil
}
