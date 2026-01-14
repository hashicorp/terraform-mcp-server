// Copyright IBM Corp. 2025
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
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
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

	permissions := org.Permissions
	humanReadablePermissions := map[string]bool{
		"Create Teams":                  permissions.CanCreateTeam,
		"Create Workspaces":             permissions.CanCreateWorkspace,
		"Create Workspace Migrations":   permissions.CanCreateWorkspaceMigration,
		"Deploy NoCode Modules":         permissions.CanDeployNoCodeModules,
		"Destroy":                       permissions.CanDestroy,
		"Manage Auditing":               permissions.CanManageAuditing,
		"Manage NoCodeModules":          permissions.CanManageNoCodeModules,
		"Manage Run Tasks":              permissions.CanManageRunTasks,
		"Traverse":                      permissions.CanTraverse,
		"Update":                        permissions.CanUpdate,
		"Update API Tokens":             permissions.CanUpdateAPIToken,
		"Update OAuth":                  permissions.CanUpdateOAuth,
		"Update Sentinel":               permissions.CanUpdateSentinel,
		"Update HYOK Configuration":     permissions.CanUpdateHYOKConfiguration,
		"View HYOK Feature Information": permissions.CanViewHYOKFeatureInfo,
		"Enable Stacks":                 permissions.CanEnableStacks,
		"Create Projects":               permissions.CanCreateProject,
	}
	perms := []string{}
	for k, v := range humanReadablePermissions {
		if v {
			perms = append(perms, k)
		}
	}

	buf, err := json.Marshal(perms)
	if err != nil {
		return ToolError(logger, "failed to marshal token permissions", err)
	}

	return mcp.NewToolResultText(string(buf)), nil
}
