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

type organizationAllowlistKey struct{}

// AuthorizeOrganization checks the resolved target, including handle-only calls.
// The middleware carries deployment policy in context; possession of a handle
// is never a substitute for this check or current backend authorization.
func AuthorizeOrganization(ctx context.Context, organization string) error {
	allowed, configured := ctx.Value(organizationAllowlistKey{}).(map[string]struct{})
	if !configured || len(allowed) == 0 {
		return nil
	}
	if _, ok := allowed[strings.ToLower(strings.TrimSpace(organization))]; !ok {
		return fmt.Errorf("organization_not_allowed")
	}
	return nil
}

func OrganizationAllowlistToolMiddleware(allowlist []string, logger *log.Logger) server.ToolHandlerMiddleware {
	allowedOrganizations := BuildAllowedOrganizationsMap(allowlist)

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx = context.WithValue(ctx, organizationAllowlistKey{}, allowedOrganizations)
			var organizationName string
			for _, alias := range []string{OrgNameArgument, "organization_name"} {
				raw, exists := request.GetArguments()[alias]
				if !exists {
					continue
				}
				value, ok := raw.(string)
				value = strings.ToLower(strings.TrimSpace(value))
				if !ok || value == "" || (organizationName != "" && organizationName != value) {
					return mcp.NewToolResultError("invalid_or_conflicting_organization_arguments"), nil
				}
				organizationName = value
			}
			if organizationName != "" {
				if err := AuthorizeOrganization(ctx, organizationName); err != nil {
					if logger != nil {
						logger.Warn("Tool target rejected by organization policy")
					}
					return mcp.NewToolResultError(err.Error()), nil
				}
			}

			return nextToolHandler(ctx, request)
		}
	}
}

// BuildAllowedOrganizationsMap builds a lookup set from allowlist
func BuildAllowedOrganizationsMap(allowlist []string) map[string]struct{} {
	allowedOrganizations := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		organizationName = strings.ToLower(strings.TrimSpace(organizationName))
		if organizationName != "" {
			allowedOrganizations[organizationName] = struct{}{}
		}
	}
	return allowedOrganizations
}
