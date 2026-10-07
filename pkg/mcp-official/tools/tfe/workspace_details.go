// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
)

// WorkspaceDetails is the bounded workspace representation returned by workspace CRUD tools.
type WorkspaceDetails struct {
	ID                  string   `json:"workspace_id"`
	Name                string   `json:"workspace_name"`
	Description         string   `json:"description"`
	AutoApply           bool     `json:"auto_apply"`
	ExecutionMode       string   `json:"execution_mode"`
	TerraformVersion    string   `json:"terraform_version"`
	WorkingDirectory    string   `json:"working_directory"`
	QueueAllRuns        bool     `json:"queue_all_runs"`
	SpeculativeEnabled  bool     `json:"speculative_enabled"`
	FileTriggersEnabled bool     `json:"file_triggers_enabled"`
	TriggerPrefixes     []string `json:"trigger_prefixes"`
	TagNames            []string `json:"tag_names"`
	ResourceCount       int      `json:"resource_count"`
	Locked              bool     `json:"locked"`
}

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
			"tag_names":             {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"resource_count":        {Type: "integer"},
			"locked":                {Type: "boolean"},
		},
		PropertyOrder: []string{
			"workspace_id", "workspace_name", "description", "auto_apply", "execution_mode",
			"terraform_version", "working_directory", "queue_all_runs", "speculative_enabled",
			"file_triggers_enabled", "trigger_prefixes", "tag_names", "resource_count", "locked",
		},
		Required: []string{
			"workspace_id", "workspace_name", "description", "auto_apply", "execution_mode",
			"terraform_version", "working_directory", "queue_all_runs", "speculative_enabled",
			"file_triggers_enabled", "trigger_prefixes", "tag_names", "resource_count", "locked",
		},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func workspaceToDetails(workspace *tfe.Workspace) WorkspaceDetails {
	if workspace == nil {
		return WorkspaceDetails{
			TriggerPrefixes: []string{},
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
		TagNames:            append([]string{}, workspace.TagNames...),
		ResourceCount:       workspace.ResourceCount,
		Locked:              workspace.Locked,
	}
}
