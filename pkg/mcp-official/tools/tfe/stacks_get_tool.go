// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StackDetails is what get_stack_details returns. tfe.Stack only has jsonapi
// tags so we map it. Stacks.Read has no include options, so the relations
// only come back with IDs.
type StackDetails struct {
	ID                         string        `json:"id"`
	Name                       string        `json:"name"`
	Description                string        `json:"description,omitempty"`
	VCSRepo                    *StackVCSRepo `json:"vcs_repo,omitempty"`
	SpeculativeEnabled         bool          `json:"speculative_enabled"`
	CreatedAt                  time.Time     `json:"created_at"`
	UpdatedAt                  time.Time     `json:"updated_at"`
	UpstreamCount              int           `json:"upstream_count"`
	DownstreamCount            int           `json:"downstream_count"`
	InputsCount                int           `json:"inputs_count"`
	OutputsCount               int           `json:"outputs_count"`
	CreationSource             string        `json:"creation_source,omitempty"`
	WorkingDirectory           string        `json:"working_directory,omitempty"`
	TriggerPatterns            []string      `json:"trigger_patterns,omitempty"`
	ProjectID                  string        `json:"project_id,omitempty"`
	AgentPoolID                string        `json:"agent_pool_id,omitempty"`
	LatestStackConfigurationID string        `json:"latest_stack_configuration_id,omitempty"`
}

// StackVCSRepo holds the version control settings for a stack.
type StackVCSRepo struct {
	Identifier        string `json:"identifier"`
	Branch            string `json:"branch,omitempty"`
	GHAInstallationID string `json:"github_app_installation_id,omitempty"`
	OAuthTokenID      string `json:"oauth_token_id,omitempty"`
}

// GetStackDetailsArguments holds the input parameters for fetching a single stack.
type GetStackDetailsArguments struct {
	// Required field
	StackID string `json:"stack_id" jsonschema:"The ID of the stack to fetch (e.g., 'st-abc123def456')"`
}

func GetStackDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_stack_details",
		Description: `Fetches detailed information about a Terraform Stack by its ID. If the stack ID isn't already known, call "list_stacks" first.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get a Terraform Stack by ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetStackDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetStackDetailsArguments) (*mcp.CallToolResult, *StackDetails, error) {
	stackID := strings.TrimSpace(input.StackID)
	if stackID == "" {
		return nil, nil, fmt.Errorf("stack_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	stack, err := tfeClient.Stacks.Read(ctx, stackID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading stack %q: %w", stackID, err)
	}

	return nil, stackToDetails(stack), nil
}

// stackToDetails maps go-tfe's Stack onto StackDetails.
func stackToDetails(stack *tfe.Stack) *StackDetails {
	if stack == nil {
		return nil
	}

	details := &StackDetails{
		ID:                 stack.ID,
		Name:               stack.Name,
		Description:        stack.Description,
		SpeculativeEnabled: stack.SpeculativeEnabled,
		CreatedAt:          stack.CreatedAt,
		UpdatedAt:          stack.UpdatedAt,
		UpstreamCount:      stack.UpstreamCount,
		DownstreamCount:    stack.DownstreamCount,
		InputsCount:        stack.InputsCount,
		OutputsCount:       stack.OutputsCount,
		CreationSource:     stack.CreationSource,
		WorkingDirectory:   stack.WorkingDirectory,
		TriggerPatterns:    stack.TriggerPatterns,
	}

	if repo := stack.VCSRepo; repo != nil {
		details.VCSRepo = &StackVCSRepo{
			Identifier:        repo.Identifier,
			Branch:            repo.Branch,
			GHAInstallationID: repo.GHAInstallationID,
			OAuthTokenID:      repo.OAuthTokenID,
		}
	}
	if stack.Project != nil {
		details.ProjectID = stack.Project.ID
	}
	if stack.AgentPool != nil {
		details.AgentPoolID = stack.AgentPool.ID
	}
	if stack.LatestStackConfiguration != nil {
		details.LatestStackConfigurationID = stack.LatestStackConfiguration.ID
	}

	return details
}
