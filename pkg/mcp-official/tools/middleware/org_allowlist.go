// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OrganizationAllowlist rejects a tools/call whose terraform_org_name argument is not in allowlist
// Tool calls without that argument are passed through unchecked
func OrganizationAllowlist(allowlist []string, logger *slog.Logger) mcp.Middleware {
	allowedOrganizations := tfeclient.BuildAllowedOrganizationsMap(allowlist)

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Only tool calls carry an organization argument to check
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok || len(params.Arguments) == 0 {
				return next(ctx, method, req)
			}

			// Arguments arrive as raw JSON on this path, pull out just the field we need instead of unmarshaling the whole tool's input
			var args struct {
				TerraformOrgName string `json:"terraform_org_name"`
			}
			if err := json.Unmarshal(params.Arguments, &args); err != nil {
				// Malformed arguments should be handled by the tool's own input validation
				return next(ctx, method, req)
			}

			organizationName := strings.ToLower(strings.TrimSpace(args.TerraformOrgName))
			if organizationName == "" {
				return next(ctx, method, req)
			}

			if _, ok := allowedOrganizations[organizationName]; !ok {
				logger.WarnContext(ctx, "rejecting tool call: organization not in allowlist",
					"tool", params.Name, "organization", organizationName)
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{
						Text: fmt.Sprintf("Terraform organization %q is not allowed by this server", organizationName),
					}},
				}, nil
			}

			return next(ctx, method, req)
		}
	}
}
