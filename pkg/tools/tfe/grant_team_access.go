// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GrantTeamAccess creates a tool to grant a team access to a workspace or project.
func GrantTeamAccess(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("grant_team_access",
			mcp.WithDescription(`Grants a team access to a workspace or project in Terraform Cloud/Enterprise. Provide either workspace_id (for workspace-level access) or project_id (for project-level access), but not both. This is a write operation that modifies access control resources.`),
			mcp.WithTitleAnnotation("Grant a team access to a workspace or project"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description("The ID of the team to grant access to (e.g., 'team-abc123def456')"),
			),
			mcp.WithString("access_level",
				mcp.Required(),
				mcp.Description("The access level to grant. For workspaces: 'admin', 'read', 'write', 'plan', or 'custom'. For projects: 'admin', 'maintain', 'write', 'read', or 'custom'."),
				mcp.Enum("admin", "maintain", "write", "read", "plan", "custom"),
			),
			mcp.WithString("workspace_id",
				mcp.Description("The ID of the workspace to grant access to (e.g., 'ws-abc123def456'). Provide this OR project_id, not both."),
			),
			mcp.WithString("project_id",
				mcp.Description("The ID of the project to grant access to (e.g., 'prj-abc123def456'). Provide this OR workspace_id, not both."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return grantTeamAccessHandler(ctx, request, logger)
		},
	}
}

func grantTeamAccessHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	teamID, err := request.RequireString("team_id")
	if err != nil {
		return ToolError(logger, "missing required input: team_id", err)
	}
	teamID = strings.TrimSpace(teamID)

	accessLevel, err := request.RequireString("access_level")
	if err != nil {
		return ToolError(logger, "missing required input: access_level", err)
	}
	accessLevel = strings.TrimSpace(accessLevel)

	workspaceID := strings.TrimSpace(request.GetString("workspace_id", ""))
	projectID := strings.TrimSpace(request.GetString("project_id", ""))

	if workspaceID == "" && projectID == "" {
		return ToolError(logger, "either workspace_id or project_id must be provided", nil)
	}
	if workspaceID != "" && projectID != "" {
		return ToolError(logger, "only one of workspace_id or project_id may be provided, not both", nil)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	if workspaceID != "" {
		return grantWorkspaceAccess(ctx, tfeClient, teamID, workspaceID, accessLevel, logger)
	}
	return grantProjectAccess(ctx, tfeClient, teamID, projectID, accessLevel, logger)
}

func grantWorkspaceAccess(ctx context.Context, tfeClient *tfe.Client, teamID, workspaceID, accessLevel string, logger *log.Logger) (*mcp.CallToolResult, error) {
	accessType := tfe.AccessType(accessLevel)
	switch accessType {
	case tfe.AccessAdmin, tfe.AccessPlan, tfe.AccessRead, tfe.AccessWrite, tfe.AccessCustom:
		// valid
	default:
		return ToolErrorf(logger, "invalid access_level '%s' for workspace access - must be one of: admin, plan, read, write, custom", accessLevel)
	}

	teamAccess, err := tfeClient.TeamAccess.Add(ctx, tfe.TeamAccessAddOptions{
		Access:    &accessType,
		Team:      &tfe.Team{ID: teamID},
		Workspace: &tfe.Workspace{ID: workspaceID},
	})
	if err != nil {
		return ToolErrorf(logger, "failed to grant team '%s' access to workspace '%s': %v", teamID, workspaceID, err)
	}

	result := map[string]interface{}{
		"access_id":    teamAccess.ID,
		"team_id":      teamID,
		"workspace_id": workspaceID,
		"access_level": string(teamAccess.Access),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal team access result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

func grantProjectAccess(ctx context.Context, tfeClient *tfe.Client, teamID, projectID, accessLevel string, logger *log.Logger) (*mcp.CallToolResult, error) {
	accessType := tfe.TeamProjectAccessType(accessLevel)
	switch accessType {
	case tfe.TeamProjectAccessAdmin, tfe.TeamProjectAccessMaintain, tfe.TeamProjectAccessWrite, tfe.TeamProjectAccessRead, tfe.TeamProjectAccessCustom:
		// valid
	default:
		return ToolErrorf(logger, "invalid access_level '%s' for project access - must be one of: admin, maintain, write, read, custom", accessLevel)
	}

	projectAccess, err := tfeClient.TeamProjectAccess.Add(ctx, tfe.TeamProjectAccessAddOptions{
		Access:  accessType,
		Team:    &tfe.Team{ID: teamID},
		Project: &tfe.Project{ID: projectID},
	})
	if err != nil {
		return ToolErrorf(logger, "failed to grant team '%s' access to project '%s': %v", teamID, projectID, err)
	}

	result := map[string]interface{}{
		"access_id":    projectAccess.ID,
		"team_id":      teamID,
		"project_id":   projectID,
		"access_level": string(projectAccess.Access),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal project access result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}
