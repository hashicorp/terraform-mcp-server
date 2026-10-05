// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetTokenPermissions creates a tool to get the list of permissions for the current TFE_TOKEN in a particular organization
func GetTokenPermissions(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_token_permissions",
			mcp.WithDescription(`Fetches the permissions the current token has for the specified terraform organization.`),
			mcp.WithTitleAnnotation("Get permissions for current token"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getTokenPermissionsHandler(ctx, req, logger)
		},
	}
}

func getTokenPermissionsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	org, err := tfeClient.Organizations.Read(ctx, terraformOrgName)
	if err != nil {
		return ToolErrorf(logger, "organization not found: %q", terraformOrgName)
	}

	buf, err := json.Marshal(client.HumanReadableTokenPermissions(org.Permissions))
	if err != nil {
		return ToolError(logger, "failed to marshal token permissions", err)
	}

	return mcp.NewToolResultText(string(buf)), nil
}
