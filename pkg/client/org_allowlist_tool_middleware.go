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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OrgNameArgument is the tool argument holding the organization name for a tool call
const OrgNameArgument = "terraform_org_name"

// OrgAttributeKey is the metric attribute that carries the Terraform organization name
const OrgAttributeKey = "terraform.org.name"

// RecordOrgToolCall counts one successful tool call against the Terraform
// organization it targeted, so dashboards can count distinct organizations
// (distinct values of the terraform.org.name attribute) using the server.
//
// It is shared by the mark3labs path (RecordToolCall) and the go-sdk path
// (pkg/mcp-official/tools/middleware.Metrics). Callers must only invoke it for
// calls that did not fail: failed calls include typos, unknown organizations
// and organizations rejected by the allowlist, none of which are real usage
// and all of which would add junk series. Tools without a terraform_org_name
// argument pass an empty orgName and are skipped
func RecordOrgToolCall(ctx context.Context, orgName string, config MetricsConfig, logger *log.Logger) {
	if !config.Enabled || config.OrgToolCallCounter == nil {
		logger.Debugf("Either metrics are not enabled or OrgToolCallCounter is NIL! Initialization failed.")
		return
	}
	orgName = NormalizeOrgName(orgName)
	if orgName == "" {
		return
	}
	config.OrgToolCallCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String(OrgAttributeKey, orgName),
		attribute.String("service.name", config.ServiceName),
		attribute.String("service.version", config.ServiceVersion),
	))
}

// NormalizeOrgName trims and lowercases an organization name so the same
// organization is never counted twice
func NormalizeOrgName(orgName string) string {
	return strings.ToLower(strings.TrimSpace(orgName))
}

func OrganizationAllowlistToolMiddleware(allowlist []string, logger *log.Logger) server.ToolHandlerMiddleware {
	allowedOrganizations := BuildAllowedOrganizationsMap(allowlist)

	return func(nextToolHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Skip allowlist enforcement for tools without an organization argument.
			organizationName := strings.ToLower(strings.TrimSpace(request.GetString(OrgNameArgument, "")))
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
