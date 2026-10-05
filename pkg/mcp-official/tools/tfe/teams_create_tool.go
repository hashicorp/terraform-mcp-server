// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var teamVisibilities = []string{"secret", "organization"}

// TeamCreateSummary is the response shape returned by create_team.
type TeamCreateSummary struct {
	ID         string `json:"team_id"`
	Name       string `json:"team_name"`
	Visibility string `json:"visibility"`
	UserCount  int    `json:"user_count,omitempty"`
}

type CreateTeamArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	TeamName         string `json:"team_name"`
	Visibility       string `json:"visibility,omitempty"`
}

func CreateTeamTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_team",
		Description: "Creates a new team in a Terraform Cloud/Enterprise organization.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"team_name": {
					Type:        "string",
					Description: "The unique name of the team to create in the Terraform organization",
				},
				"visibility": {
					Type:        "string",
					Description: "Team visibility. If omitted the API defaults to secret",
					Enum:        enumOf(teamVisibilities...),
				},
			},
			PropertyOrder:        []string{"terraform_org_name", "team_name", "visibility"},
			Required:             []string{"terraform_org_name", "team_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new team in a Terraform organization",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateTeamFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateTeamArguments) (*mcp.CallToolResult, *TeamCreateSummary, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	teamName := strings.TrimSpace(input.TeamName)
	if teamName == "" {
		return nil, nil, fmt.Errorf("team_name must not be blank")
	}

	visibility := strings.ToLower(strings.TrimSpace(input.Visibility))
	if visibility != "" && !slices.Contains(teamVisibilities, visibility) {
		return nil, nil, fmt.Errorf("invalid visibility %q, must be one of: %s", visibility, strings.Join(teamVisibilities, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	options := tfe.TeamCreateOptions{
		Name: tfe.String(teamName),
	}
	if visibility != "" {
		options.Visibility = tfe.String(visibility)
	}

	team, err := tfeClient.Teams.Create(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("creating team %q in organization %q: %w", teamName, terraformOrgName, err)
	}

	return nil, &TeamCreateSummary{
		ID:         team.ID,
		Name:       team.Name,
		Visibility: team.Visibility,
		UserCount:  team.UserCount,
	}, nil
}
