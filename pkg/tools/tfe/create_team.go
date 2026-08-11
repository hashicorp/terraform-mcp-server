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
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var validTeamVisibilities = []string{"secret", "organization"}
var validTeamVisibilitiesStr = strings.Join(validTeamVisibilities, ", ")

type TeamSummary struct {
	ID         string `json:"team_id"`
	Name       string `json:"team_name"`
	Visibility string `json:"visibility"`
	UserCount  int    `json:"user_count,omitempty"`
}

// CreateTeam creates a tool to create a new team in a Terraform Cloud/Enterprise organization.
func CreateTeam(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_team",
			mcp.WithDescription(`Creates a new team in a Terraform Cloud/Enterprise organization.`),
			mcp.WithTitleAnnotation("Create a new team in a Terraform organization"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription),
			),
			mcp.WithString("team_name",
				mcp.Required(),
				mcp.Description("The unique name of the team to create in the Terraform organization"),
			),
			mcp.WithString("visibility",
				mcp.Description("Optional team visibility: "+validTeamVisibilitiesStr+`. If omitted, the API defaults to "secret".`),
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

	visibility := strings.ToLower(GetTrimmedString(request, "visibility", ""))
	if visibility != "" && !slices.Contains(validTeamVisibilities, visibility) {
		return ToolErrorf(logger, "invalid visibility %q - must be one of: %s", visibility, validTeamVisibilitiesStr)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	options := tfe.TeamCreateOptions{
		Name: tfe.String(teamName),
	}

	if visibility != "" {
		options.Visibility = tfe.String(visibility)
	}

	team, err := tfeClient.Teams.Create(ctx, terraformOrgName, options)
	if err != nil {
		return ToolErrorf(logger, "failed to create team %q in org %q: %v", teamName, terraformOrgName, err)
	}

	teamJSON, err := json.Marshal(&TeamSummary{
		team.ID,
		team.Name,
		team.Visibility,
		team.UserCount,
	})
	if err != nil {
		return ToolError(logger, "failed to marshal created team summary", err)
	}

	return mcp.NewToolResultText(string(teamJSON)), nil
}
