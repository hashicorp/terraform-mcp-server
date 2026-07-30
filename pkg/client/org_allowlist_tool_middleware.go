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
	allowedOrganizations := buildAllowedOrganizationsMap(allowlist)

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Skip allowlist enforcement for tools without an organization argument.
			organizationName := strings.ToLower(strings.TrimSpace(request.GetString(orgNameArgument, "")))
			if organizationName == "" {
				return nextToolHandler(ctx, request)
			}

			if _, allowed := allowedOrganizations[organizationName]; !allowed {
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
func buildAllowedOrganizationsMap(allowlist []string) map[string]struct{} {
	allowedOrganizations := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		if organizationName != "" {
			allowedOrganizations[organizationName] = struct{}{}
		}
	}
	return allowedOrganizations
}
