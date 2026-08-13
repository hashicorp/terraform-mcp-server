// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GrantTeamAccess creates a tool to grant team access to a given workspace or project.
func GrantTeamAccess(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"grant_team_access",
			mcp.WithDescription(`Grants a team permission to access a workspace or a project in Terraform Cloud/Enterprise.
			Provide either workspace_id (for workspace-level access) or project_id (for project-level access) — not both.
			Returns the created access grant including its ID, team ID, target resource ID, and access level.`),
			mcp.WithTitleAnnotation("Grant team access to a workspace or project"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description(`The ID of the team to grant access. Team IDs begin with 'team-' (e.g., 'team-abc123def456').`),
			),
			mcp.WithString("access_level",
				mcp.Required(),
				mcp.Description(`The permission level to grant the team.
				For workspace access (workspace_id): "read" (view only), "plan" (can queue plans), "write" (apply runs), "admin" (full control).
				For project access (project_id): "read", "write", "maintain" (manage workspaces), "admin" (full control). Note: "plan" is only valid for workspaces; "maintain" is only valid for projects.`),
			),
			mcp.WithString("workspace_id",
				mcp.Description(`The ID of the workspace to grant the team access to. Workspace IDs begin with 'ws-' (e.g., 'ws-abc123def456'). Mutually exclusive with project_id — provide one or the other, not both.`),
			),
			mcp.WithString("project_id",
				mcp.Description(`The ID of the project to grant the team access to. Project IDs begin with 'prj-' (e.g., 'prj-abc123def456'). Mutually exclusive with workspace_id — provide one or the other, not both.`),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return grantTeamAccessHandler(ctx, request, logger)
		},
	}
}

var validTeamAccessLevels = []string{"admin", "read", "write", "plan"}
var validTeamAccessLevelsStr = strings.Join(validTeamAccessLevels, ", ")
var validTeamProjectAccessLevels = []string{"admin", "read", "write", "maintain"}
var validTeamProjectAccessLevelsStr = strings.Join(validTeamProjectAccessLevels, ", ")

// grantTeamAccessHandler handles tool logics and functionality
func grantTeamAccessHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	teamID, err := request.RequireString("team_id")
	if err != nil {
		return ToolError(logger, "Missing required input: team_id", err)
	}
	accessLevel, err := request.RequireString("access_level")
	if err != nil {
		return ToolError(logger, "Missing required input: access_level", err)
	}
	workspaceID := GetTrimmedString(request, "workspace_id", "")
	projectID := GetTrimmedString(request, "project_id", "")
	teamID = strings.TrimSpace(teamID)
	accessLevel = strings.ToLower(strings.TrimSpace(accessLevel))

	if workspaceID == "" && projectID == "" {
		return ToolError(logger, "One of workspace_id or project_id must be provided", nil)
	}

	if workspaceID != "" && projectID != "" {
		return ToolError(logger, "Only one of workspace_id or project_id may be provided, not both", nil)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	if workspaceID != "" {
		if !slices.Contains(validTeamAccessLevels, accessLevel) {
			return ToolErrorf(logger, "Invalid Team access level %q - must be one of: %s", accessLevel, validTeamAccessLevelsStr)
		}

		ta, err := tfeClient.TeamAccess.Add(ctx, tfe.TeamAccessAddOptions{
			Access:    tfe.Access(tfe.AccessType(accessLevel)),
			Workspace: &tfe.Workspace{ID: workspaceID},
			Team:      &tfe.Team{ID: teamID},
		})
		if err != nil {
			return ToolErrorf(logger, "Failed to grant team access to workspace %q: %v", workspaceID, err)
		}

		summaryJSON, err := json.Marshal(TeamAccessSummary{
			ID:          ta.ID,
			TeamID:      ta.Team.ID,
			WorkspaceID: ta.Workspace.ID,
			Access:      string(ta.Access),
		})
		if err != nil {
			return ToolError(logger, "Failed to serialize summary", err)
		}
		return mcp.NewToolResultText(string(summaryJSON)), nil
	}

	if !slices.Contains(validTeamProjectAccessLevels, accessLevel) {
		return ToolErrorf(logger, "Invalid Team Project access level %q - must be one of: %s", accessLevel, validTeamProjectAccessLevelsStr)
	}

	tpa, err := tfeClient.TeamProjectAccess.Add(ctx, tfe.TeamProjectAccessAddOptions{
		Access:  tfe.TeamProjectAccessType(accessLevel),
		Project: &tfe.Project{ID: projectID},
		Team:    &tfe.Team{ID: teamID},
	})
	if err != nil {
		return ToolErrorf(logger, "Failed to grant team project access to project %q: %v", projectID, err)
	}

	summaryJSON, err := json.Marshal(TeamProjectAccessSummary{
		ID:        tpa.ID,
		TeamID:    tpa.Team.ID,
		ProjectID: tpa.Project.ID,
		Access:    string(tpa.Access),
	})
	if err != nil {
		return ToolError(logger, "Failed to serialize summary", err)
	}
	return mcp.NewToolResultText(string(summaryJSON)), nil
}

// TeamAccessSummary is the response summary for a granted workspace team access
type TeamAccessSummary struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	WorkspaceID string `json:"workspace_id"`
	Access      string `json:"access"`
}

// TeamProjectAccessSummary is the response summary for a granted project team access
type TeamProjectAccessSummary struct {
	ID        string `json:"id"`
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
	Access    string `json:"access"`
}
