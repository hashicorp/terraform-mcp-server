// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DeleteTeamArguments holds the input parameters for deleting a team.
type DeleteTeamArguments struct {
	TeamID string `json:"team_id" jsonschema:"The ID of the team to delete (e.g., 'team-abc123def456')"`
}

// DeleteTeamResponse is the response shape returned by the delete_team tool.
type DeleteTeamResponse struct {
	ID      string `json:"team_id"`
	Deleted bool   `json:"deleted"`
}

// DeleteTeamTool describes the delete_team tool.
func DeleteTeamTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "delete_team",
		Description: `Permanently deletes a Terraform team by its "team_id". This also removes all team memberships and any workspace or project access granted through the team. Organization users and workspaces are not deleted. If only a team name is known, use the list_teams tool first to look up the team_id.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           `Delete a Terraform team by "team_id"`,
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func DeleteTeamFunc(ctx context.Context, request *mcp.CallToolRequest, input DeleteTeamArguments) (*mcp.CallToolResult, *DeleteTeamResponse, error) {
	teamID := strings.TrimSpace(input.TeamID)
	if teamID == "" {
		return nil, nil, fmt.Errorf("team_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if err := tfeClient.Teams.Delete(ctx, teamID); err != nil {
		return nil, nil, fmt.Errorf("deleting team %q: %w", teamID, err)
	}

	return nil, &DeleteTeamResponse{ID: teamID, Deleted: true}, nil
}
