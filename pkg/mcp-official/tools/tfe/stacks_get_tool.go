// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StackDetails is what get_stack_details returns. tfe.Stack only has jsonapi
// tags so we map it. Stacks.Read has no include options, so the relations
// only come back with IDs. field names match tfe.Stack so the drift test
// can line them up.
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
	TriggerPatterns            []string      `json:"trigger_patterns"`
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
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"id":          {Type: "string"},
				"name":        {Type: "string"},
				"description": {Type: "string"},
				"vcs_repo": {
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"identifier":                 {Type: "string"},
						"branch":                     {Type: "string"},
						"github_app_installation_id": {Type: "string"},
						"oauth_token_id":             {Type: "string"},
					},
					Required:             []string{"identifier"},
					AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
				},
				"speculative_enabled":           {Type: "boolean"},
				"created_at":                    {Type: "string", Format: "date-time"},
				"updated_at":                    {Type: "string", Format: "date-time"},
				"upstream_count":                {Type: "integer"},
				"downstream_count":              {Type: "integer"},
				"inputs_count":                  {Type: "integer"},
				"outputs_count":                 {Type: "integer"},
				"creation_source":               {Type: "string"},
				"working_directory":             {Type: "string"},
				"trigger_patterns":              {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
				"project_id":                    {Type: "string"},
				"agent_pool_id":                 {Type: "string"},
				"latest_stack_configuration_id": {Type: "string"},
			},
			Required:             []string{"id", "name", "speculative_enabled", "created_at", "updated_at", "upstream_count", "downstream_count", "inputs_count", "outputs_count", "trigger_patterns"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get a Terraform Stack by ID",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
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

// stackToDetails maps go-tfe's Stack onto StackDetails. kept separate from
// GetStackDetailsFunc so the mapping can be tested without a TFE client.
func stackToDetails(stack *tfe.Stack) *StackDetails {
	if stack == nil {
		return nil
	}

	// always non-nil so it marshals as [] not null, matching the schema
	triggerPatterns := make([]string, 0, len(stack.TriggerPatterns))
	triggerPatterns = append(triggerPatterns, stack.TriggerPatterns...)

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
		TriggerPatterns:    triggerPatterns,
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
