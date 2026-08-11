// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"strings"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetTeam creates a tool to fetch full details for a single team by ID.
func GetTeam(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_team",
			mcp.WithDescription(`Fetch full details for a single team by ID, including members, organization access permissions, and SSO settings.`),
			mcp.WithTitleAnnotation("Fetch full details for a single team by ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("team_id",
				mcp.Required(),
				mcp.Description("The ID of the team to retrieve (e.g. team-abc123)."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getTeamHandler(ctx, request, logger)
		},
	}
}

func getTeamHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	teamID, err := request.RequireString("team_id")
	if err != nil {
		return ToolError(logger, "missing required input: team_id", err)
	}
	teamID = strings.TrimSpace(teamID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	team, err := tfeClient.Teams.Read(ctx, teamID)
	if err != nil {
		return ToolErrorf(logger, "failed to read team %q: %v", teamID, err)
	}

	buf := bytes.NewBuffer(nil)
	if err := jsonapi.MarshalPayloadWithoutIncluded(buf, team); err != nil {
		return ToolError(logger, "failed to marshal team details", err)
	}
	return mcp.NewToolResultText(buf.String()), nil
}
