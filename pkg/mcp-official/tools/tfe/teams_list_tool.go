// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TeamSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	UserCount  int    `json:"users-count"`
}

// TeamSummaryList is a list of team summaries and pagination details
type TeamSummaryList struct {
	Items []TeamSummary `json:"items"`
	PaginationDetails
}

// schema is declared in ListTeamsTool, so no jsonschema tags here.
type ListTeamsArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	TeamNames        string `json:"team_names,omitempty"`
	SearchQuery      string `json:"search_query,omitempty"`
	Pagination
}

func ListTeamsTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Terraform Cloud/Enterprise organization",
	}
	properties["team_names"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Comma-separated list of exact team names to filter by. Only teams whose name exactly matches one of the provided values are returned. Example: owners,developers,platform-infra",
	}
	properties["search_query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Substring search query to filter teams by name. Returns all teams whose name contains the query string. Example: platform",
	}

	return &mcp.Tool{
		Name:        "list_teams",
		Description: `Search and list teams within a specified Terraform Cloud/Enterprise organization. Returns a truncated summary of each team, use "get_team" to get the full details for a specific team. Optionally filter by exact team names or a substring search query. Supports pagination for large result sets.`,
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "team_names", "search_query", "page", "pageSize"},
			Required:             []string{"terraform_org_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"id":          {Type: "string"},
							"name":        {Type: "string"},
							"visibility":  {Type: "string"},
							"users-count": {Type: "integer"},
						},
						PropertyOrder:        []string{"id", "name", "visibility", "users-count"},
						Required:             []string{"id", "name", "visibility", "users-count"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
				"current-page": {Type: "integer"},
				"prev-page":    {Type: "integer"},
				"next-page":    {Type: "integer"},
				"total-count":  {Type: "integer"},
				"total-pages":  {Type: "integer"},
			},
			PropertyOrder:        []string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"},
			Required:             []string{"items"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "List teams in a Terraform organization",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListTeamsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListTeamsArguments) (*mcp.CallToolResult, *TeamSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	var teamNames []string
	if names := strings.TrimSpace(input.TeamNames); names != "" {
		teamNames = strings.Split(names, ",")
		for i, name := range teamNames {
			teamNames[i] = strings.TrimSpace(name)
		}
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	teams, err := tfeClient.Teams.List(ctx, terraformOrgName, &tfe.TeamListOptions{
		Names:       teamNames,
		Query:       strings.TrimSpace(input.SearchQuery),
		ListOptions: input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing teams in organization %q: %w", terraformOrgName, err)
	}
	if len(teams.Items) == 0 {
		return nil, nil, fmt.Errorf("no teams to list in organization %q", terraformOrgName)
	}

	summaries := make([]TeamSummary, len(teams.Items))
	for i, t := range teams.Items {
		summaries[i] = TeamSummary{
			ID:         t.ID,
			Name:       t.Name,
			Visibility: t.Visibility,
			UserCount:  t.UserCount,
		}
	}

	return nil, &TeamSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(teams.Pagination),
	}, nil
}
