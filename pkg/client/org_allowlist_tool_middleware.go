// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// OrgNameArgument is the tool argument holding the organization name for a tool call
const OrgNameArgument = "terraform_org_name"

type organizationAllowlistContextKey struct{}

func WithOrganizationAllowlist(ctx context.Context, allowedOrganizations map[string]struct{}) context.Context {
	return context.WithValue(ctx, organizationAllowlistContextKey{}, allowedOrganizations)
}

func OrganizationAllowed(ctx context.Context, organizationName string) bool {
	allowedOrganizations, ok := ctx.Value(organizationAllowlistContextKey{}).(map[string]struct{})
	if !ok {
		return true
	}
	_, allowed := allowedOrganizations[strings.ToLower(strings.TrimSpace(organizationName))]
	return allowed
}

func OrganizationAllowlistToolMiddleware(allowlist []string, logger *log.Logger) server.ToolHandlerMiddleware {
	allowedOrganizations := BuildAllowedOrganizationsMap(allowlist)

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx = WithOrganizationAllowlist(ctx, allowedOrganizations)

			// Tools that pass the organization by argument are checked here; ID-based
			// tools carry no name and are checked in their handlers.
			organizationName := strings.ToLower(strings.TrimSpace(request.GetString(OrgNameArgument, "")))
			if organizationName != "" && !OrganizationAllowed(ctx, organizationName) {
				logger.Warnf("Rejecting tool call %q: organization %q is not in the configured allowlist",
					request.Params.Name, organizationName)
				return mcp.NewToolResultError(fmt.Sprintf(
					"Terraform organization %q is not allowed by this server", organizationName)), nil
			}

			return nextToolHandler(ctx, request)
		}
	}
}

// BuildAllowedOrganizationsMap builds a lookup set from allowlist
func BuildAllowedOrganizationsMap(allowlist []string) map[string]struct{} {
	allowedOrganizations := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		if organizationName != "" {
			allowedOrganizations[organizationName] = struct{}{}
		}
	}
	return allowedOrganizations
}
