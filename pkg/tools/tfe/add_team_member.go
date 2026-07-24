// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// AddTeamMemeber creates a tool to add a new member to your Terraform team.
func AddTeamMemeber(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"add_team_member",
			mcp.WithDescription("Adds member’s to a team. This is a write operation"),
			mcp.WithTitleAnnotation(`Adds member’s to a team`),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description("The Teams's id (e.g., 'team-abc123def456')"),
			),
			mcp.WithString("username",
				mcp.Description("Optional: Comma-separated list of usernames to add (e.g., 'alice' or 'alice, bob'). Only works for users who have accepted the organization invite."),
			),
			mcp.WithString("organization_membership_ids",
				mcp.Description("Optional: Comma-separated list of organization membership IDs to add (e.g., 'ou-abc123' or 'ou-abc123, ou-def456'). Works for both accepted and pending organization invites. Prefer this over 'username' when the invitee has not yet accepted."),
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
	username := request.GetString("username", "")
	organizationMembershipID := request.GetString("organization_membership_ids", "")

	teamID = strings.TrimSpace(teamID)
	username = strings.TrimSpace((username))
	organizationMembershipID = strings.TrimSpace(organizationMembershipID)

	var usernames []string
	if username != "" {
		usernames = strings.Split(username, ",")
		for i, u := range usernames {
			usernames[i] = strings.TrimSpace(u)
		}
	}
	var organizationMembershipIDs []string
	if organizationMembershipID != "" {
		organizationMembershipIDs = strings.Split(organizationMembershipID, ",")
		for i, id := range organizationMembershipIDs {
			organizationMembershipIDs[i] = strings.TrimSpace(id)
		}
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	if len(usernames) == 0 && len(organizationMembershipIDs) == 0 {
		return ToolError(logger, "At least one of 'username' or 'organization_membership_ids' must be provided", nil)
	}

	result := &AddMemberSummary{}

	if len(usernames) > 0 {
		if err := tfeClient.TeamMembers.Add(ctx, teamID, tfe.TeamMemberAddOptions{
			Usernames: usernames,
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to add by username: %v", err))
		} else {
			result.AddedByUsername = usernames
		}
	}

	if len(organizationMembershipIDs) > 0 {
		if err := tfeClient.TeamMembers.Add(ctx, teamID, tfe.TeamMemberAddOptions{
			OrganizationMembershipIDs: organizationMembershipIDs,
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to add by membership ID: %v", err))
		} else {
			result.AddedByMembershipID = organizationMembershipIDs
		}
	}

	result.Success = len(result.Errors) == 0

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "Failed to marshal result", err)
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}

// AddMemberSummary is a truncated summary of Added Members details for listing
type AddMemberSummary struct {
	Success             bool     `json:"success"`
	AddedByUsername     []string `json:"added_by_username,omitempty"`
	AddedByMembershipID []string `json:"added_by_membership_id,omitempty"`
	Errors              []string `json:"errors,omitempty"`
}
