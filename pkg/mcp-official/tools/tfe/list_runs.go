// Copyright IBM Corp. 2025
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

// Default pagination applied when the caller omits page/pageSize, matching the
// defaults the mark3labs implementation got from utils.OptionalPaginationParams.
const (
	defaultPageNumber = 1
	defaultPageSize   = 30
)

// runStatuses is the set of run states accepted by the status filter.
// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/run#run-states
var runStatuses = []any{
	"pending",
	"fetching",
	"fetching_completed",
	"pre_plan_running",
	"pre_plan_completed",
	"queuing",
	"plan_queued",
	"planning",
	"planned",
	"cost_estimating",
	"cost_estimated",
	"policy_checking",
	"policy_override",
	"policy_soft_failed",
	"policy_checked",
	"confirmed",
	"post_plan_running",
	"post_plan_completed",
	"planned_and_finished",
	"planned_and_saved",
	"apply_queued",
	"applying",
	"applied",
	"discarded",
	"errored",
	"canceled",
	"force_canceled",
}

// RunSummary is a truncated summary of a Run for top level listing
type RunSummary struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
	HasChanges    bool      `json:"has_changes"`
	IsDestroy     bool      `json:"is_destroy"`
	PlanOnly      bool      `json:"plan_only"`
	RefreshOnly   bool      `json:"refresh_only"`
	WorkspaceName string    `json:"workspace_name"`
}

// RunSummaryList contains the list of run summaries and pagination details
type RunSummaryList struct {
	Items []*RunSummary `json:"items"`
	*tfe.Pagination
}

// ListRunsArguments is the unmarshal target for list_runs. The wire contract -
// descriptions, status enum, pagination bounds and which fields are required - is
// declared by the input schema in ListRunsTool, so these fields carry no
// jsonschema tags.
type ListRunsArguments struct {
	TerraformOrgName string   `json:"terraform_org_name"`
	WorkspaceName    string   `json:"workspace_name,omitempty"`
	VCSUsername      string   `json:"vcs_username,omitempty"`
	Status           []string `json:"status,omitempty"`
	Page             int      `json:"page,omitempty"`
	PageSize         int      `json:"pageSize,omitempty"`
}

// ListRunsTool creates a tool to list Terraform runs in an organization or workspace.
func ListRunsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_runs",
		Description: "List or search Terraform runs in a specific workspace with optional filtering.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform runs",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: terraformOrgNameDescription + " in which to list runs based on the provided filters when no workspace is specified",
				},
				"workspace_name": {
					Type:        "string",
					Description: "If specified, lists the runs in the given workspace instead of the organization based on filters",
				},
				"vcs_username": {
					Type:        "string",
					Description: "Searches for runs that match the VCS username you supply",
				},
				"status": {
					Type:        "array",
					Description: "Optional run status filter",
					Items: &jsonschema.Schema{
						Type: "string",
						Enum: runStatuses,
					},
				},
				"page": {
					Type:        "integer",
					Description: "Page number for pagination (min 1)",
					Minimum:     jsonschema.Ptr(1.0),
				},
				"pageSize": {
					Type:        "integer",
					Description: "Results per page for pagination (min 1, max 100)",
					Minimum:     jsonschema.Ptr(1.0),
					Maximum:     jsonschema.Ptr(100.0),
				},
			},
			Required: []string{"terraform_org_name"},
		},
	}
}

// ListRunsFunc lists runs for a workspace when workspace_name is supplied, and
// for the whole organization otherwise.
func ListRunsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListRunsArguments) (*mcp.CallToolResult, *RunSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("missing required input: terraform_org_name")
	}
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	vcsUsername := strings.TrimSpace(input.VCSUsername)

	// tfe expects a comma-separated list of run states.
	status := strings.Join(input.Status, ",")

	listOptions := tfe.ListOptions{
		PageNumber: input.Page,
		PageSize:   input.PageSize,
	}
	if listOptions.PageNumber == 0 {
		listOptions.PageNumber = defaultPageNumber
	}
	if listOptions.PageSize == 0 {
		listOptions.PageSize = defaultPageSize
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	if workspaceName != "" {
		workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
		if err != nil {
			return nil, nil, fmt.Errorf("workspace '%s' not found in org '%s': %w", workspaceName, terraformOrgName, err)
		}

		runs, err := tfeClient.Runs.List(ctx, workspace.ID, &tfe.RunListOptions{
			ListOptions: listOptions,
			Status:      status,
			User:        vcsUsername,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list runs in workspace '%s': %w", workspaceName, err)
		}

		return nil, &RunSummaryList{
			Items:      runSummaries(runs.Items),
			Pagination: runs.Pagination,
		}, nil
	}

	runs, err := tfeClient.Runs.ListForOrganization(ctx, terraformOrgName, &tfe.RunListForOrganizationOptions{
		ListOptions: listOptions,
		Status:      status,
		User:        vcsUsername,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list runs in org '%s': %w", terraformOrgName, err)
	}

	// ListForOrganization only reports next/previous, not a total page count.
	pagination := &tfe.Pagination{}
	if runs.PaginationNextPrev != nil {
		pagination.CurrentPage = runs.PaginationNextPrev.CurrentPage
		pagination.PreviousPage = runs.PaginationNextPrev.PreviousPage
		pagination.NextPage = runs.PaginationNextPrev.NextPage
	}

	return nil, &RunSummaryList{
		Items:      runSummaries(runs.Items),
		Pagination: pagination,
	}, nil
}

// runSummaries trims a list of runs down to the fields exposed by list_runs.
func runSummaries(runs []*tfe.Run) []*RunSummary {
	summaries := make([]*RunSummary, len(runs))
	for i, r := range runs {
		summaries[i] = &RunSummary{
			ID:          r.ID,
			Status:      string(r.Status),
			Message:     r.Message,
			Source:      string(r.Source),
			CreatedAt:   r.CreatedAt,
			HasChanges:  r.HasChanges,
			IsDestroy:   r.IsDestroy,
			PlanOnly:    r.PlanOnly,
			RefreshOnly: r.RefreshOnly,
		}
		// The workspace relationship is not always side-loaded.
		if r.Workspace != nil {
			summaries[i].WorkspaceName = r.Workspace.Name
		}
	}
	return summaries
}
