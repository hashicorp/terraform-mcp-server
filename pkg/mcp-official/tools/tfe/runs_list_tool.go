// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var runStatuses = []string{
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

// ListRunsArguments holds filters for listing runs in a workspace or organization.
type ListRunsArguments struct {
	TerraformOrgName string   `json:"terraform_org_name"`
	WorkspaceName    string   `json:"workspace_name,omitempty"`
	VCSUsername      string   `json:"vcs_username,omitempty"`
	Status           []string `json:"status,omitempty"`
	Pagination
}

// RunSummary is the bounded run representation returned by list_runs.
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

type RunSummaryList struct {
	Items []RunSummary `json:"items"`
	PaginationDetails
}

// ListRunsTool describes the list_runs tool.
func ListRunsTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The Terraform Cloud/Enterprise organization in which to list runs",
	}
	properties["workspace_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional workspace name; when omitted, runs are listed across the organization",
	}
	properties["vcs_username"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional VCS username filter",
	}
	properties["status"] = &jsonschema.Schema{
		Type:        "array",
		Description: "Optional run status filters",
		Items:       &jsonschema.Schema{Type: "string", Enum: enumOf(runStatuses...)},
	}

	return &mcp.Tool{
		Name:        "list_runs",
		Description: "Lists or searches Terraform runs in a workspace or organization with optional filtering.",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "vcs_username", "status", "page", "pageSize"},
			Required:             []string{"terraform_org_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: runSummaryListSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform runs",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListRunsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListRunsArguments) (*mcp.CallToolResult, *RunSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	for _, status := range input.Status {
		if !slices.Contains(runStatuses, status) {
			return nil, nil, fmt.Errorf("invalid status %q", status)
		}
	}

	status := strings.Join(input.Status, ",")
	workspaceName := strings.TrimSpace(input.WorkspaceName)

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if workspaceName != "" {
		workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
		if err != nil {
			return nil, nil, fmt.Errorf("reading workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
		}

		runs, err := tfeClient.Runs.List(ctx, workspace.ID, &tfe.RunListOptions{
			ListOptions: input.ListOptions(),
			Include:     []tfe.RunIncludeOpt{tfe.RunWorkspace},
			Status:      status,
			User:        input.VCSUsername,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("listing runs in workspace %q: %w", workspaceName, err)
		}
		if runs == nil {
			return nil, nil, fmt.Errorf("listing runs in workspace %q: Terraform returned an empty response", workspaceName)
		}

		return nil, &RunSummaryList{
			Items:             runSummaries(runs.Items),
			PaginationDetails: paginationDetails(runs.Pagination),
		}, nil
	}

	runs, err := tfeClient.Runs.ListForOrganization(ctx, terraformOrgName, &tfe.RunListForOrganizationOptions{
		ListOptions: input.ListOptions(),
		Include:     []tfe.RunIncludeOpt{tfe.RunWorkspace},
		Status:      status,
		User:        input.VCSUsername,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing runs in organization %q: %w", terraformOrgName, err)
	}
	if runs == nil {
		return nil, nil, fmt.Errorf("listing runs in organization %q: Terraform returned an empty response", terraformOrgName)
	}

	return nil, &RunSummaryList{
		Items:             runSummaries(runs.Items),
		PaginationDetails: runPaginationDetails(runs.PaginationNextPrev),
	}, nil
}

func runSummaries(runs []*tfe.Run) []RunSummary {
	summaries := make([]RunSummary, 0, len(runs))
	for _, run := range runs {
		if run == nil {
			continue
		}

		summary := RunSummary{
			ID:          run.ID,
			Status:      string(run.Status),
			Message:     run.Message,
			Source:      string(run.Source),
			CreatedAt:   run.CreatedAt,
			HasChanges:  run.HasChanges,
			IsDestroy:   run.IsDestroy,
			PlanOnly:    run.PlanOnly,
			RefreshOnly: run.RefreshOnly,
		}
		if run.Workspace != nil {
			summary.WorkspaceName = run.Workspace.Name
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func runPaginationDetails(pagination *tfe.PaginationNextPrev) PaginationDetails {
	if pagination == nil {
		return PaginationDetails{}
	}
	return PaginationDetails{
		CurrentPage:  pagination.CurrentPage,
		PreviousPage: pagination.PreviousPage,
		NextPage:     pagination.NextPage,
	}
}

func runSummaryListSchema() *jsonschema.Schema {
	itemProperties := map[string]*jsonschema.Schema{
		"id":             {Type: "string"},
		"status":         {Type: "string"},
		"message":        {Type: "string"},
		"source":         {Type: "string"},
		"created_at":     {Type: "string", Format: "date-time"},
		"has_changes":    {Type: "boolean"},
		"is_destroy":     {Type: "boolean"},
		"plan_only":      {Type: "boolean"},
		"refresh_only":   {Type: "boolean"},
		"workspace_name": {Type: "string"},
	}
	itemFields := []string{"id", "status", "message", "source", "created_at", "has_changes", "is_destroy", "plan_only", "refresh_only", "workspace_name"}

	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"items": {
				Type: "array",
				Items: &jsonschema.Schema{
					Type:                 "object",
					Properties:           itemProperties,
					PropertyOrder:        itemFields,
					Required:             itemFields,
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
	}
}
