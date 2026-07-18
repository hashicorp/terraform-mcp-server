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

// CreateTeam creates a tool to create a new team in a Terraform organization.
func CreateTeam(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_team",
			mcp.WithDescription(`Creates a new team in the specified Terraform Cloud/Enterprise organization. Teams are used to manage access to projects and workspaces. This is a write operation that modifies org-level resources.`),
			mcp.WithTitleAnnotation("Create a new Terraform team"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("team_name",
				mcp.Required(),
				mcp.Description("The name of the team to create"),
			),
			mcp.WithString("visibility",
				mcp.Description("Team visibility: 'organization' (visible to all org members) or 'secret' (visible only to members and owners). Defaults to 'organization'."),
				mcp.Enum("organization", "secret"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createTeamHandler(ctx, request, logger)
		},
	}
}

func createTeamHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	teamName, err := request.RequireString("team_name")
	if err != nil {
		return ToolError(logger, "missing required input: team_name", err)
	}
	teamName = strings.TrimSpace(teamName)

	visibility := request.GetString("visibility", "organization")
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		visibility = "organization"
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	options := tfe.TeamCreateOptions{
		Name:       tfe.String(teamName),
		Visibility: tfe.String(visibility),
	}

	team, err := tfeClient.Teams.Create(ctx, terraformOrgName, options)
	if err != nil {
		return ToolErrorf(logger, "failed to create team '%s' in org '%s': %v", teamName, terraformOrgName, err)
	}

	result := &TeamSummary{
		ID:         team.ID,
		Name:       team.Name,
		Visibility: team.Visibility,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal team result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// TeamSummary is a truncated summary of team details
type TeamSummary struct {
	ID         string `json:"team_id"`
	Name       string `json:"team_name"`
	Visibility string `json:"visibility"`
}
