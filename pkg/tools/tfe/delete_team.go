// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// DeleteTeam creates a tool to delete a Terraform team by ID
// Team ID can be found at:
// https://app.terraform.io/app/your-organization/settings/teams/team-xxxxxxxx
func DeleteTeam(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"delete_team",
			mcp.WithDescription(`Permanently deletes a Terraform team by its team_id.  If you don't have the team_id, look it up first via the organization's Teams URL page from the HCP Terraform/TFE UI.`),
			mcp.WithTitleAnnotation("Deletes a Terraform Team by team_id"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description("The ID of the team to delete (e.g., 'team-abc123def456')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return deleteTeamHandler(ctx, request, logger)
		},
	}

}

// deleteTeamHandler handles tool logics and functionality
func deleteTeamHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {

	// Init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Params Required
	teamID, err := request.RequireString("team_id")
	if err != nil {
		return ToolError(logger, "Missing required input: team_id", err)
	}
	// Handle White Spaces
	teamID = strings.TrimSpace(teamID)

	// Read ID
	team, err := tfeClient.Teams.Read(ctx, teamID)

	// Delete Team
	err = tfeClient.Teams.Delete(ctx, teamID)
	if err != nil {
		return ToolErrorf(logger, "Failed to delete team '%s' %v", teamID, err)
	}

}
