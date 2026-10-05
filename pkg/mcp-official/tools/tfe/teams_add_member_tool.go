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

// AddTeamMemberResult is the response shape returned by add_team_member.
type AddTeamMemberResult struct {
	TeamID string `json:"team_id"`
	Added  string `json:"added"`
}

// schema is declared in AddTeamMemberTool, so no jsonschema tags here.
type AddTeamMemberArguments struct {
	TeamID                   string `json:"team_id"`
	Username                 string `json:"username,omitempty"`
	OrganizationMembershipID string `json:"organization_membership_id,omitempty"`
}

func AddTeamMemberTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "add_team_member",
		Description: "Adds a single member to a Terraform Cloud/Enterprise team. Provide either a username (accepted invites only) or an organization membership ID (accepted and pending invites), not both.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"team_id": {
					Type:        "string",
					Description: "The ID of the Terraform Cloud/Enterprise team to add members to (e.g. team-abc123def456)",
				},
				"username": {
					Type:        "string",
					Description: "Username of the member to add. Only works for users who have accepted the organization invite",
				},
				"organization_membership_id": {
					Type:        "string",
					Description: "Organization membership ID of the member to add (e.g. ou-abc123). Works for both accepted and pending organization invites. Prefer this over username when the invitee has not yet accepted",
				},
			},
			PropertyOrder:        []string{"team_id", "username", "organization_membership_id"},
			Required:             []string{"team_id"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Add member to a Terraform team",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func AddTeamMemberFunc(ctx context.Context, request *mcp.CallToolRequest, input AddTeamMemberArguments) (*mcp.CallToolResult, *AddTeamMemberResult, error) {
	teamID := strings.TrimSpace(input.TeamID)
	if teamID == "" {
		return nil, nil, fmt.Errorf("team_id must not be blank")
	}

	username := strings.TrimSpace(input.Username)
	orgMembershipID := strings.TrimSpace(input.OrganizationMembershipID)
	if username == "" && orgMembershipID == "" {
		return nil, nil, fmt.Errorf("one of username or organization_membership_id must be provided")
	}
	if username != "" && orgMembershipID != "" {
		return nil, nil, fmt.Errorf("provide only one of username or organization_membership_id, not both")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
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
		return nil, nil, fmt.Errorf("adding member %q to team %q: %w", memberID, teamID, err)
	}

	return nil, &AddTeamMemberResult{
		TeamID: teamID,
		Added:  memberID,
	}, nil
}
