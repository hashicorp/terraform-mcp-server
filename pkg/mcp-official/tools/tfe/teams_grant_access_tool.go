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

// plan is workspace only, maintain is project only.
var (
	teamWorkspaceAccessLevels = []string{"admin", "read", "write", "plan"}
	teamProjectAccessLevels   = []string{"admin", "read", "write", "maintain"}
	teamAllAccessLevels       = []string{"admin", "read", "write", "plan", "maintain"}
)

// TeamAccessGrant is the response shape returned by grant_team_access.
type TeamAccessGrant struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Access      string `json:"access"`
}

// schema is declared in GrantTeamAccessTool, so no jsonschema tags here.
type GrantTeamAccessArguments struct {
	TeamID      string `json:"team_id"`
	AccessLevel string `json:"access_level"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
}

func GrantTeamAccessTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "grant_team_access",
		Description: "Grants a team permission to access a workspace or a project in Terraform Cloud/Enterprise. Provide either workspace_id (for workspace-level access) or project_id (for project-level access), not both. Returns the created access grant including its ID, team ID, target resource ID, and access level.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"team_id": {
					Type:        "string",
					Description: "The ID of the team to grant access. Team IDs begin with team- (e.g. team-abc123def456)",
				},
				"access_level": {
					Type:        "string",
					Description: "The permission level to grant the team. For workspace access: read, plan, write, admin. For project access: read, write, maintain, admin. plan is only valid for workspaces and maintain is only valid for projects",
					Enum:        enumOf(teamAllAccessLevels...),
				},
				"workspace_id": {
					Type:        "string",
					Description: "The ID of the workspace to grant the team access to. Workspace IDs begin with ws- (e.g. ws-abc123def456). Mutually exclusive with project_id",
				},
				"project_id": {
					Type:        "string",
					Description: "The ID of the project to grant the team access to. Project IDs begin with prj- (e.g. prj-abc123def456). Mutually exclusive with workspace_id",
				},
			},
			PropertyOrder:        []string{"team_id", "access_level", "workspace_id", "project_id"},
			Required:             []string{"team_id", "access_level"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Grant team access to a workspace or project",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GrantTeamAccessFunc(ctx context.Context, request *mcp.CallToolRequest, input GrantTeamAccessArguments) (*mcp.CallToolResult, *TeamAccessGrant, error) {
	teamID := strings.TrimSpace(input.TeamID)
	if teamID == "" {
		return nil, nil, fmt.Errorf("team_id must not be blank")
	}
	accessLevel := strings.ToLower(strings.TrimSpace(input.AccessLevel))
	if accessLevel == "" {
		return nil, nil, fmt.Errorf("access_level must not be blank")
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	projectID := strings.TrimSpace(input.ProjectID)

	if workspaceID == "" && projectID == "" {
		return nil, nil, fmt.Errorf("one of workspace_id or project_id must be provided")
	}
	if workspaceID != "" && projectID != "" {
		return nil, nil, fmt.Errorf("only one of workspace_id or project_id may be provided, not both")
	}
	if workspaceID != "" && !slices.Contains(teamWorkspaceAccessLevels, accessLevel) {
		return nil, nil, fmt.Errorf("invalid workspace access level %q, must be one of: %s", accessLevel, strings.Join(teamWorkspaceAccessLevels, ", "))
	}
	if projectID != "" && !slices.Contains(teamProjectAccessLevels, accessLevel) {
		return nil, nil, fmt.Errorf("invalid project access level %q, must be one of: %s", accessLevel, strings.Join(teamProjectAccessLevels, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if workspaceID != "" {
		access, err := tfeClient.TeamAccess.Add(ctx, tfe.TeamAccessAddOptions{
			Access:    tfe.Access(tfe.AccessType(accessLevel)),
			Workspace: &tfe.Workspace{ID: workspaceID},
			Team:      &tfe.Team{ID: teamID},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("granting team access to workspace %q: %w", workspaceID, err)
		}
		return nil, &TeamAccessGrant{
			ID:          access.ID,
			TeamID:      access.Team.ID,
			WorkspaceID: access.Workspace.ID,
			Access:      string(access.Access),
		}, nil
	}

	projectAccess, err := tfeClient.TeamProjectAccess.Add(ctx, tfe.TeamProjectAccessAddOptions{
		Access:  tfe.TeamProjectAccessType(accessLevel),
		Project: &tfe.Project{ID: projectID},
		Team:    &tfe.Team{ID: teamID},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("granting team access to project %q: %w", projectID, err)
	}
	return nil, &TeamAccessGrant{
		ID:        projectAccess.ID,
		TeamID:    projectAccess.Team.ID,
		ProjectID: projectAccess.Project.ID,
		Access:    string(projectAccess.Access),
	}, nil
}
