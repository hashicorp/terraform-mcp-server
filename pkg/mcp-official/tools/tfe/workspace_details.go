// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
)

// WorkspaceDetails is the bounded workspace representation returned by workspace CRUD tools.
// All fields are populated from the single API response — no extra API calls are needed.
// For tag bindings, state versions, or the current run use the dedicated tools instead.
type WorkspaceDetails struct {
	ID                  string    `json:"workspace_id"`
	Name                string    `json:"workspace_name"`
	Description         string    `json:"description"`
	AutoApply           bool      `json:"auto_apply"`
	ExecutionMode       string    `json:"execution_mode"`
	TerraformVersion    string    `json:"terraform_version"`
	WorkingDirectory    string    `json:"working_directory"`
	QueueAllRuns        bool      `json:"queue_all_runs"`
	SpeculativeEnabled  bool      `json:"speculative_enabled"`
	FileTriggersEnabled bool      `json:"file_triggers_enabled"`
	TriggerPrefixes     []string  `json:"trigger_prefixes"`
	TriggerPatterns     []string  `json:"trigger_patterns"`
	TagNames            []string  `json:"tag_names"`
	ResourceCount       int       `json:"resource_count"`
	Locked              bool      `json:"locked"`
	AssessmentsEnabled  bool      `json:"assessments_enabled"`
	GlobalRemoteState   bool      `json:"global_remote_state"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// workspaceDetailsSchema returns the shared OutputSchema for workspace CRUD tools.
// It is used directly by create_workspace and update_workspace, and extended by
// get_workspace_details which adds the variables and readme fields on top.
func workspaceDetailsSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"workspace_id":          {Type: "string"},
			"workspace_name":        {Type: "string"},
			"description":           {Type: "string"},
			"auto_apply":            {Type: "boolean"},
			"execution_mode":        {Type: "string"},
			"terraform_version":     {Type: "string"},
			"working_directory":     {Type: "string"},
			"queue_all_runs":        {Type: "boolean"},
			"speculative_enabled":   {Type: "boolean"},
			"file_triggers_enabled": {Type: "boolean"},
			"trigger_prefixes":      {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"trigger_patterns":      {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"tag_names":             {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"resource_count":        {Type: "integer"},
			"locked":                {Type: "boolean"},
			"assessments_enabled":   {Type: "boolean"},
			"global_remote_state":   {Type: "boolean"},
			"created_at":            {Type: "string", Format: "date-time"},
			"updated_at":            {Type: "string", Format: "date-time"},
		},
		PropertyOrder: []string{
			"workspace_id", "workspace_name", "description", "auto_apply", "execution_mode",
			"terraform_version", "working_directory", "queue_all_runs", "speculative_enabled",
			"file_triggers_enabled", "trigger_prefixes", "trigger_patterns", "tag_names",
			"resource_count", "locked", "assessments_enabled", "global_remote_state",
			"created_at", "updated_at",
		},
		Required: []string{
			"workspace_id", "workspace_name", "description", "auto_apply", "execution_mode",
			"terraform_version", "working_directory", "queue_all_runs", "speculative_enabled",
			"file_triggers_enabled", "trigger_prefixes", "trigger_patterns", "tag_names",
			"resource_count", "locked", "assessments_enabled", "global_remote_state",
			"created_at", "updated_at",
		},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// workspaceToDetails maps a tfe.Workspace API response onto WorkspaceDetails.
// It ensures slice fields are always non-nil so they serialise as [] rather than null.
func workspaceToDetails(workspace *tfe.Workspace) WorkspaceDetails {
	if workspace == nil {
		return WorkspaceDetails{
			TriggerPrefixes: []string{},
			TriggerPatterns: []string{},
			TagNames:        []string{},
		}
	}

	return WorkspaceDetails{
		ID:                  workspace.ID,
		Name:                workspace.Name,
		Description:         workspace.Description,
		AutoApply:           workspace.AutoApply,
		ExecutionMode:       workspace.ExecutionMode,
		TerraformVersion:    workspace.TerraformVersion,
		WorkingDirectory:    workspace.WorkingDirectory,
		QueueAllRuns:        workspace.QueueAllRuns,
		SpeculativeEnabled:  workspace.SpeculativeEnabled,
		FileTriggersEnabled: workspace.FileTriggersEnabled,
		TriggerPrefixes:     append([]string{}, workspace.TriggerPrefixes...),
		TriggerPatterns:     append([]string{}, workspace.TriggerPatterns...),
		TagNames:            append([]string{}, workspace.TagNames...),
		ResourceCount:       workspace.ResourceCount,
		Locked:              workspace.Locked,
		AssessmentsEnabled:  workspace.AssessmentsEnabled,
		GlobalRemoteState:   workspace.GlobalRemoteState,
		CreatedAt:           workspace.CreatedAt,
		UpdatedAt:           workspace.UpdatedAt,
	}
}
