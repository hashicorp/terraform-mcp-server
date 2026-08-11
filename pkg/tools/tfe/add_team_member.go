// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// AddTeamMember creates a tool to add a new member to your Terraform team.
func AddTeamMember(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"add_team_member",
			mcp.WithDescription("Adds a single member to a Terraform Cloud/Enterprise team. Provide either a username (accepted invites only) or an organization membership ID (accepted and pending invites), not both."),
			mcp.WithTitleAnnotation(`Add member to a Terraform team`),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description("The ID of the Terraform Cloud/Enterprise team to add members to (e.g., 'team-abc123def456')"),
			),
			mcp.WithString("username",
				mcp.Description("Username of the member to add. Only works for users who have accepted the organization invite."),
			),
			mcp.WithString("organization_membership_id",
				mcp.Description("Organization membership ID of the member to add (e.g., 'ou-abc123'). Works for both accepted and pending organization invites. Prefer this over 'username' when the invitee has not yet accepted."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return addTeamMemberHandler(ctx, request, logger)
		},
	}
}

// addTeamMemberHandler handles tool logics and functionality
func addTeamMemberHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	teamID, err := request.RequireString("team_id")
	if err != nil {
		return ToolError(logger, "Missing required input: team_id", err)
	}
	username := GetTrimmedString(request, "username", "")
	orgMembershipID := GetTrimmedString(request, "organization_membership_id", "")
	teamID = strings.TrimSpace(teamID)

	if username == "" && orgMembershipID == "" {
		return ToolError(logger, "One of 'username' or 'organization_membership_id' must be provided", nil)
	}
	if username != "" && orgMembershipID != "" {
		return ToolError(logger, "Provide only one of 'username' or 'organization_membership_id', not both", nil)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	options := tfe.TeamMemberAddOptions{}
	var memberID string
	if username != "" {
		options.Usernames = []string{username}
		memberID = username
	} else {
		options.OrganizationMembershipIDs = []string{orgMembershipID}
		memberID = orgMembershipID
	}
	if err := tfeClient.TeamMembers.Add(ctx, teamID, options); err != nil {
		return ToolError(logger, fmt.Sprintf("Failed to add member %q to team %q", memberID, teamID), err)
	}
	return mcp.NewToolResultText(fmt.Sprintf("Successfully added member to team %q", teamID)), nil
}
