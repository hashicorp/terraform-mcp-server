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
			mcp.WithDescription("Grant a team access to a workspace or project"),
			mcp.WithTitleAnnotation("Grant team access"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description(`The ID of the Terraform Cloud/Enterprise team to grant access to (e.g., 'team-abc123def456')`),
			),
			mcp.WithString("access_level",
				mcp.Required(),
				mcp.Description(`The access level to grant access for (e.g. "admin", "read", "write", "plan", "custom")`),
			),
			mcp.WithString("workspace_id",
				mcp.Description(`The ID of the Terraform Cloud/Enterprise workspace to grant access to (e.g., "ws-abc123def456")`),
			),
			mcp.WithString("project_id",
				mcp.Description(`The ID of the Terraform Cloud/Enterprise project to grant access to (e.g., "prj-abc123def456")`),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return grantTeamAccessHandler(ctx, request, logger)
		},
	}
}

var validTeamAccessLevels = []string{"admin", "read", "write", "plan", "custom"}
var validTeamAccessLevelsStr = strings.Join(validTeamAccessLevels, ", ")
var validTeamProjectAccessLevels = []string{"admin", "read", "write", "maintain", "custom"}
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
	workspaceID := request.GetString("workspace_id", "")
	projectID := request.GetString("project_id", "")

	teamID = strings.TrimLeft(strings.TrimSpace(teamID), "/")
	workspaceID = strings.TrimSpace(workspaceID)
	projectID = strings.TrimSpace(projectID)
	accessLevel = strings.ToLower(strings.TrimSpace(accessLevel))

	// At least one must be provided, not neither
	if workspaceID == "" && projectID == "" {
		return ToolError(logger, "One of workspace_id or project_id must be provided", nil)
	}

	// Only one must be provided, not both
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

		workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
		if err != nil {
			return ToolErrorf(logger, "Workspace %q not found: %v", workspaceID, err)
		}

		ta, err := tfeClient.TeamAccess.Add(ctx, tfe.TeamAccessAddOptions{
			Access:    tfe.Access(tfe.AccessType(accessLevel)),
			Workspace: workspace,
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
			return ToolError(logger, "failed to serialize summary", err)
		}

		return mcp.NewToolResultText(string(summaryJSON)), nil
	}

	if !slices.Contains(validTeamProjectAccessLevels, accessLevel) {
		return ToolErrorf(logger, "Invalid Team Project access level %q - must be one of: %s", accessLevel, validTeamProjectAccessLevelsStr)
	}

	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return ToolErrorf(logger, "Project %q not found: %v", projectID, err)
	}

	tpa, err := tfeClient.TeamProjectAccess.Add(ctx, tfe.TeamProjectAccessAddOptions{
		Access:  tfe.TeamProjectAccessType(accessLevel),
		Project: project,
		Team:    &tfe.Team{ID: teamID},
	})
	if err != nil {
		return ToolErrorf(logger, "Failed to grant team project access to project %q: %v", workspaceID, err)
	}

	summaryJSON, err := json.Marshal(TeamProjectAccessSummary{
		ID:        tpa.ID,
		TeamID:    tpa.Team.ID,
		ProjectID: tpa.Project.ID,
		Access:    string(tpa.Access),
	})
	if err != nil {
		return ToolError(logger, "failed to serialize summary", err)
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
