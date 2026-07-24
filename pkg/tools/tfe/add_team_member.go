// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"

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
				mcp.Description("The Teams's id (e.g., 'team-abc123def456')"),
			),
			mcp.WithString("username",
				mcp.Description("The Member's username"),
			),
			mcp.WithString("organization_membership_ids",
				mcp.Description("The Org. membership's id"),
			),
		),

		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return addTeamMemberHandler(ctx, request, logger)

		},
	}
}

// addTeamMemberHandler handles tool logics and functionality
func addTeamMemberHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {

	return mcp.NewToolResultText(""), nil
}
