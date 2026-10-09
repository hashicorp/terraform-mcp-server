// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// OrgNameArgument is the tool argument holding the organization name for a tool call
const OrgNameArgument = "terraform_org_name"

// SearchOrgNameArgument is the organization argument used by the Search tools
const SearchOrgNameArgument = "organization_name"

var organizationArguments = []string{OrgNameArgument, SearchOrgNameArgument}

// ErrOrganizationNotAllowed is returned when an organization is outside the configured MCP allowlist
var ErrOrganizationNotAllowed = errors.New("organization is not allowed by this server")

type organizationAllowlistContextKey struct{}

func OrganizationAllowlistToolMiddleware(allowlist []string, logger *log.Logger) server.ToolHandlerMiddleware {
	allowedOrganizations := BuildAllowedOrganizationsMap(allowlist)

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Expose the policy to handlers that must authorize organizations resolved from opaque IDs.
			ctx = WithOrganizationAllowlist(ctx, allowedOrganizations)

			organizationName, err := resolveOrganizationArgument(request.GetArguments())
			if err != nil {
				logger.Warnf("Rejecting tool call %q: %v", request.Params.Name, err)
				return mcp.NewToolResultError(fmt.Sprintf("Invalid organization argument: %v", err)), nil
			}
			// Tools without an organization argument are authorized by their own handlers, if needed.
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

// resolveOrganizationArgument returns the normalized organization named by the supported
// organization arguments. Non-string values and conflicting aliases are rejected.
func resolveOrganizationArgument(arguments map[string]any) (string, error) {
	var organizationName string
	for _, argument := range organizationArguments {
		value, ok := arguments[argument]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", argument)
		}
		normalized := normalizeOrganizationName(text)
		if normalized == "" {
			continue
		}
		if organizationName != "" && organizationName != normalized {
			return "", fmt.Errorf("%s conflicts with another organization argument", argument)
		}
		organizationName = normalized
	}
	return organizationName, nil
}

func normalizeOrganizationName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// BuildAllowedOrganizationsMap builds a lookup set from allowlist
func BuildAllowedOrganizationsMap(allowlist []string) map[string]struct{} {
	allowedOrganizations := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		if organizationName = normalizeOrganizationName(organizationName); organizationName != "" {
			allowedOrganizations[organizationName] = struct{}{}
		}
	}
	return allowedOrganizations
}

// WithOrganizationAllowlist stores the configured organization allowlist in ctx.
func WithOrganizationAllowlist(ctx context.Context, allowedOrganizations map[string]struct{}) context.Context {
	return context.WithValue(ctx, organizationAllowlistContextKey{}, allowedOrganizations)
}

func allowlistFromContext(ctx context.Context) (map[string]struct{}, bool) {
	allowedOrganizations, ok := ctx.Value(organizationAllowlistContextKey{}).(map[string]struct{})
	return allowedOrganizations, ok
}

// AuthorizeOrganization checks organizationName against the allowlist carried by ctx.
// It succeeds when no allowlist is configured, and fails closed on an empty name otherwise.
func AuthorizeOrganization(ctx context.Context, organizationName string) error {
	allowedOrganizations, ok := allowlistFromContext(ctx)
	if !ok {
		return nil
	}
	if _, allowed := allowedOrganizations[normalizeOrganizationName(organizationName)]; !allowed {
		return ErrOrganizationNotAllowed
	}
	return nil
}

// OrganizationAllowlistConfigured reports whether ctx carries an organization allowlist.
func OrganizationAllowlistConfigured(ctx context.Context) bool {
	_, ok := allowlistFromContext(ctx)
	return ok
}
