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

// GetRunDetailsArguments holds the run ID to read.
type GetRunDetailsArguments struct {
	RunID string `json:"run_id" jsonschema:"The ID of the run to get details for"`
}

type RunActionDetails struct {
	IsCancelable      bool `json:"is_cancelable"`
	IsConfirmable     bool `json:"is_confirmable"`
	IsDiscardable     bool `json:"is_discardable"`
	IsForceCancelable bool `json:"is_force_cancelable"`
}

type RunPermissionDetails struct {
	CanApply        bool `json:"can_apply"`
	CanCancel       bool `json:"can_cancel"`
	CanDiscard      bool `json:"can_discard"`
	CanForceCancel  bool `json:"can_force_cancel"`
	CanForceExecute bool `json:"can_force_execute"`
}

type RunStatusTimestampDetails struct {
	AppliedAt            string `json:"applied_at,omitempty"`
	ApplyingAt           string `json:"applying_at,omitempty"`
	ApplyQueuedAt        string `json:"apply_queued_at,omitempty"`
	CanceledAt           string `json:"canceled_at,omitempty"`
	ConfirmedAt          string `json:"confirmed_at,omitempty"`
	CostEstimatedAt      string `json:"cost_estimated_at,omitempty"`
	CostEstimatingAt     string `json:"cost_estimating_at,omitempty"`
	DiscardedAt          string `json:"discarded_at,omitempty"`
	ErroredAt            string `json:"errored_at,omitempty"`
	FetchedAt            string `json:"fetched_at,omitempty"`
	FetchingAt           string `json:"fetching_at,omitempty"`
	ForceCanceledAt      string `json:"force_canceled_at,omitempty"`
	PlannedAndFinishedAt string `json:"planned_and_finished_at,omitempty"`
	PlannedAndSavedAt    string `json:"planned_and_saved_at,omitempty"`
	PlannedAt            string `json:"planned_at,omitempty"`
	PlanningAt           string `json:"planning_at,omitempty"`
	PlanQueueableAt      string `json:"plan_queueable_at,omitempty"`
	PlanQueuedAt         string `json:"plan_queued_at,omitempty"`
	PolicyCheckedAt      string `json:"policy_checked_at,omitempty"`
	PolicySoftFailedAt   string `json:"policy_soft_failed_at,omitempty"`
	PostPlanCompletedAt  string `json:"post_plan_completed_at,omitempty"`
	PostPlanRunningAt    string `json:"post_plan_running_at,omitempty"`
	PrePlanCompletedAt   string `json:"pre_plan_completed_at,omitempty"`
	PrePlanRunningAt     string `json:"pre_plan_running_at,omitempty"`
	QueuingAt            string `json:"queuing_at,omitempty"`
}

// RunDetails contains the run attributes returned by the base runs endpoint.
// Run variables are intentionally omitted because the API provides no sensitivity metadata.
type RunDetails struct {
	ID                     string                     `json:"id"`
	Actions                *RunActionDetails          `json:"actions,omitempty"`
	AutoApply              bool                       `json:"auto_apply"`
	AllowConfigGeneration  *bool                      `json:"allow_config_generation,omitempty"`
	AllowEmptyApply        bool                       `json:"allow_empty_apply"`
	CanceledAt             string                     `json:"canceled_at,omitempty"`
	CreatedAt              string                     `json:"created_at"`
	ForceCancelAvailableAt string                     `json:"force_cancel_available_at,omitempty"`
	HasChanges             bool                       `json:"has_changes"`
	IsDestroy              bool                       `json:"is_destroy"`
	InvokeActionAddrs      []string                   `json:"invoke_action_addrs"`
	Message                string                     `json:"message"`
	Permissions            *RunPermissionDetails      `json:"permissions,omitempty"`
	PolicyPaths            []string                   `json:"policy_paths"`
	PositionInQueue        int                        `json:"position_in_queue"`
	PlanOnly               bool                       `json:"plan_only"`
	Refresh                bool                       `json:"refresh"`
	RefreshOnly            bool                       `json:"refresh_only"`
	ReplaceAddrs           []string                   `json:"replace_addrs"`
	SavePlan               bool                       `json:"save_plan"`
	Source                 string                     `json:"source"`
	Status                 string                     `json:"status"`
	StatusTimestamps       *RunStatusTimestampDetails `json:"status_timestamps,omitempty"`
	TargetAddrs            []string                   `json:"target_addrs"`
	TerraformVersion       string                     `json:"terraform_version"`
	TriggerReason          string                     `json:"trigger_reason"`
	ApplyID                string                     `json:"apply_id,omitempty"`
	ConfigurationVersionID string                     `json:"configuration_version_id,omitempty"`
	CostEstimateID         string                     `json:"cost_estimate_id,omitempty"`
	CreatedByID            string                     `json:"created_by_id,omitempty"`
	ConfirmedByID          string                     `json:"confirmed_by_id,omitempty"`
	PlanID                 string                     `json:"plan_id,omitempty"`
	WorkspaceID            string                     `json:"workspace_id,omitempty"`
}

// GetRunDetailsTool describes the get_run_details tool.
func GetRunDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "get_run_details",
		Description:  "Fetches detailed information about a specific Terraform run.",
		OutputSchema: runDetailsSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a Terraform run",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetRunDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetRunDetailsArguments) (*mcp.CallToolResult, *RunDetails, error) {
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return nil, nil, fmt.Errorf("run_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	run, err := tfeClient.Runs.Read(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading run %q: %w", runID, err)
	}
	if run == nil {
		return nil, nil, fmt.Errorf("reading run %q: Terraform returned an empty response", runID)
	}

	details := runToDetails(run)
	return nil, &details, nil
}

func runToDetails(run *tfe.Run) RunDetails {
	details := RunDetails{
		InvokeActionAddrs: []string{},
		PolicyPaths:       []string{},
		ReplaceAddrs:      []string{},
		TargetAddrs:       []string{},
	}
	if run == nil {
		return details
	}

	details.ID = run.ID
	details.AutoApply = run.AutoApply
	details.AllowConfigGeneration = run.AllowConfigGeneration
	details.AllowEmptyApply = run.AllowEmptyApply
	details.CanceledAt = runTime(run.CanceledAt)
	details.CreatedAt = runTime(run.CreatedAt)
	details.ForceCancelAvailableAt = runTime(run.ForceCancelAvailableAt)
	details.HasChanges = run.HasChanges
	details.IsDestroy = run.IsDestroy
	details.InvokeActionAddrs = append(details.InvokeActionAddrs, run.InvokeActionAddrs...)
	details.Message = run.Message
	details.PolicyPaths = append(details.PolicyPaths, run.PolicyPaths...)
	details.PositionInQueue = run.PositionInQueue
	details.PlanOnly = run.PlanOnly
	details.Refresh = run.Refresh
	details.RefreshOnly = run.RefreshOnly
	details.ReplaceAddrs = append(details.ReplaceAddrs, run.ReplaceAddrs...)
	details.SavePlan = run.SavePlan
	details.Source = string(run.Source)
	details.Status = string(run.Status)
	details.TargetAddrs = append(details.TargetAddrs, run.TargetAddrs...)
	details.TerraformVersion = run.TerraformVersion
	details.TriggerReason = run.TriggerReason

	if run.Actions != nil {
		details.Actions = &RunActionDetails{
			IsCancelable:      run.Actions.IsCancelable,
			IsConfirmable:     run.Actions.IsConfirmable,
			IsDiscardable:     run.Actions.IsDiscardable,
			IsForceCancelable: run.Actions.IsForceCancelable,
		}
	}
	if run.Permissions != nil {
		details.Permissions = &RunPermissionDetails{
			CanApply:        run.Permissions.CanApply,
			CanCancel:       run.Permissions.CanCancel,
			CanDiscard:      run.Permissions.CanDiscard,
			CanForceCancel:  run.Permissions.CanForceCancel,
			CanForceExecute: run.Permissions.CanForceExecute,
		}
	}
	if run.StatusTimestamps != nil {
		details.StatusTimestamps = runStatusTimestamps(run.StatusTimestamps)
	}
	if run.Apply != nil {
		details.ApplyID = run.Apply.ID
	}
	if run.ConfigurationVersion != nil {
		details.ConfigurationVersionID = run.ConfigurationVersion.ID
	}
	if run.CostEstimate != nil {
		details.CostEstimateID = run.CostEstimate.ID
	}
	if run.CreatedBy != nil {
		details.CreatedByID = run.CreatedBy.ID
	}
	if run.ConfirmedBy != nil {
		details.ConfirmedByID = run.ConfirmedBy.ID
	}
	if run.Plan != nil {
		details.PlanID = run.Plan.ID
	}
	if run.Workspace != nil {
		details.WorkspaceID = run.Workspace.ID
	}

	return details
}

func runStatusTimestamps(timestamps *tfe.RunStatusTimestamps) *RunStatusTimestampDetails {
	return &RunStatusTimestampDetails{
		AppliedAt:            runTime(timestamps.AppliedAt),
		ApplyingAt:           runTime(timestamps.ApplyingAt),
		ApplyQueuedAt:        runTime(timestamps.ApplyQueuedAt),
		CanceledAt:           runTime(timestamps.CanceledAt),
		ConfirmedAt:          runTime(timestamps.ConfirmedAt),
		CostEstimatedAt:      runTime(timestamps.CostEstimatedAt),
		CostEstimatingAt:     runTime(timestamps.CostEstimatingAt),
		DiscardedAt:          runTime(timestamps.DiscardedAt),
		ErroredAt:            runTime(timestamps.ErroredAt),
		FetchedAt:            runTime(timestamps.FetchedAt),
		FetchingAt:           runTime(timestamps.FetchingAt),
		ForceCanceledAt:      runTime(timestamps.ForceCanceledAt),
		PlannedAndFinishedAt: runTime(timestamps.PlannedAndFinishedAt),
		PlannedAndSavedAt:    runTime(timestamps.PlannedAndSavedAt),
		PlannedAt:            runTime(timestamps.PlannedAt),
		PlanningAt:           runTime(timestamps.PlanningAt),
		PlanQueueableAt:      runTime(timestamps.PlanQueueableAt),
		PlanQueuedAt:         runTime(timestamps.PlanQueuedAt),
		PolicyCheckedAt:      runTime(timestamps.PolicyCheckedAt),
		PolicySoftFailedAt:   runTime(timestamps.PolicySoftFailedAt),
		PostPlanCompletedAt:  runTime(timestamps.PostPlanCompletedAt),
		PostPlanRunningAt:    runTime(timestamps.PostPlanRunningAt),
		PrePlanCompletedAt:   runTime(timestamps.PrePlanCompletedAt),
		PrePlanRunningAt:     runTime(timestamps.PrePlanRunningAt),
		QueuingAt:            runTime(timestamps.QueuingAt),
	}
}

func runTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func runDetailsSchema() *jsonschema.Schema {
	stringArray := func() *jsonschema.Schema {
		return &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Type: "string"}}
	}
	optionalDateTime := func() *jsonschema.Schema {
		return &jsonschema.Schema{Type: "string", Format: "date-time"}
	}

	properties := map[string]*jsonschema.Schema{
		"id":                        {Type: "string"},
		"actions":                   runActionDetailsSchema(),
		"auto_apply":                {Type: "boolean"},
		"allow_config_generation":   {Type: "boolean"},
		"allow_empty_apply":         {Type: "boolean"},
		"canceled_at":               optionalDateTime(),
		"created_at":                optionalDateTime(),
		"force_cancel_available_at": optionalDateTime(),
		"has_changes":               {Type: "boolean"},
		"is_destroy":                {Type: "boolean"},
		"invoke_action_addrs":       stringArray(),
		"message":                   {Type: "string"},
		"permissions":               runPermissionDetailsSchema(),
		"policy_paths":              stringArray(),
		"position_in_queue":         {Type: "integer"},
		"plan_only":                 {Type: "boolean"},
		"refresh":                   {Type: "boolean"},
		"refresh_only":              {Type: "boolean"},
		"replace_addrs":             stringArray(),
		"save_plan":                 {Type: "boolean"},
		"source":                    {Type: "string"},
		"status":                    {Type: "string"},
		"status_timestamps":         runStatusTimestampDetailsSchema(),
		"target_addrs":              stringArray(),
		"terraform_version":         {Type: "string"},
		"trigger_reason":            {Type: "string"},
		"apply_id":                  {Type: "string"},
		"configuration_version_id":  {Type: "string"},
		"cost_estimate_id":          {Type: "string"},
		"created_by_id":             {Type: "string"},
		"confirmed_by_id":           {Type: "string"},
		"plan_id":                   {Type: "string"},
		"workspace_id":              {Type: "string"},
	}

	required := []string{
		"id", "auto_apply", "allow_empty_apply", "created_at", "has_changes", "is_destroy",
		"invoke_action_addrs", "message", "policy_paths", "position_in_queue", "plan_only",
		"refresh", "refresh_only", "replace_addrs", "save_plan", "source", "status",
		"target_addrs", "terraform_version", "trigger_reason",
	}
	order := []string{
		"id", "actions", "auto_apply", "allow_config_generation", "allow_empty_apply", "canceled_at",
		"created_at", "force_cancel_available_at", "has_changes", "is_destroy", "invoke_action_addrs",
		"message", "permissions", "policy_paths", "position_in_queue", "plan_only", "refresh",
		"refresh_only", "replace_addrs", "save_plan", "source", "status", "status_timestamps",
		"target_addrs", "terraform_version", "trigger_reason", "apply_id", "configuration_version_id",
		"cost_estimate_id", "created_by_id", "confirmed_by_id", "plan_id", "workspace_id",
	}

	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		PropertyOrder:        order,
		Required:             required,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func runActionDetailsSchema() *jsonschema.Schema {
	fields := []string{"is_cancelable", "is_confirmable", "is_discardable", "is_force_cancelable"}
	properties := make(map[string]*jsonschema.Schema, len(fields))
	for _, field := range fields {
		properties[field] = &jsonschema.Schema{Type: "boolean"}
	}
	return &jsonschema.Schema{Type: "object", Properties: properties, PropertyOrder: fields, Required: fields, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}

func runPermissionDetailsSchema() *jsonschema.Schema {
	fields := []string{"can_apply", "can_cancel", "can_discard", "can_force_cancel", "can_force_execute"}
	properties := make(map[string]*jsonschema.Schema, len(fields))
	for _, field := range fields {
		properties[field] = &jsonschema.Schema{Type: "boolean"}
	}
	return &jsonschema.Schema{Type: "object", Properties: properties, PropertyOrder: fields, Required: fields, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}

func runStatusTimestampDetailsSchema() *jsonschema.Schema {
	fields := []string{
		"applied_at", "applying_at", "apply_queued_at", "canceled_at", "confirmed_at",
		"cost_estimated_at", "cost_estimating_at", "discarded_at", "errored_at", "fetched_at",
		"fetching_at", "force_canceled_at", "planned_and_finished_at", "planned_and_saved_at",
		"planned_at", "planning_at", "plan_queueable_at", "plan_queued_at", "policy_checked_at",
		"policy_soft_failed_at", "post_plan_completed_at", "post_plan_running_at",
		"pre_plan_completed_at", "pre_plan_running_at", "queuing_at",
	}
	properties := make(map[string]*jsonschema.Schema, len(fields))
	for _, field := range fields {
		properties[field] = &jsonschema.Schema{Type: "string", Format: "date-time"}
	}
	return &jsonschema.Schema{Type: "object", Properties: properties, PropertyOrder: fields, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}
