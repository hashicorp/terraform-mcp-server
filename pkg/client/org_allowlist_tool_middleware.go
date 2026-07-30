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

const orgNameArgument = "terraform_org_name"

func OrganizationAllowlistToolMiddleware(allowlist []string, logger *log.Logger) server.ToolHandlerMiddleware {
	allowedOrganizations := OrganizationAllowlistSet(allowlist)

	// if no allowed organization listed, skip to next toolmiddleware without need to initialize OrganizationAllowlistToolMiddleware
	if len(allowedOrganizations) == 0 {
		return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
			return nextToolHandler
		}
	}

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// if tool doesn't have orgNameArgument, skipped without need to check
			organizationName := strings.TrimSpace(request.GetString(orgNameArgument, ""))
			if organizationName == "" {
				return nextToolHandler(ctx, request)
			}

			if _, allowed := allowedOrganizations[strings.ToLower(organizationName)]; !allowed {
				logger.Warnf("Rejecting tool call %q: organization %q is not in the configured allowlist",
					request.Params.Name, organizationName)
				return mcp.NewToolResultError(fmt.Sprintf(
					"Terraform organization %q is not allowed by this server", organizationName)), nil
			}

			return nextToolHandler(ctx, request)
		}
	}
}

// builds a lookup set from allowlist
func OrganizationAllowlistSet(allowlist []string) map[string]struct{} {
	allowedSet := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		if organizationName != "" {
			allowedSet[organizationName] = struct{}{}
		}
	}
	return allowedSet
}
