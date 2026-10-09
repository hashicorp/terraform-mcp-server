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

// UpdateWorkspaceArguments holds the inputs for updating a workspace.
type UpdateWorkspaceArguments struct {
	TerraformOrgName    string   `json:"terraform_org_name"`
	WorkspaceName       string   `json:"workspace_name"`
	NewName             string   `json:"new_name,omitempty"`
	Description         *string  `json:"description,omitempty"`
	TerraformVersion    string   `json:"terraform_version,omitempty"`
	WorkingDirectory    *string  `json:"working_directory,omitempty"`
	AutoApply           *bool    `json:"auto_apply,omitempty"`
	ExecutionMode       string   `json:"execution_mode,omitempty"`
	QueueAllRuns        *bool    `json:"queue_all_runs,omitempty"`
	SpeculativeEnabled  *bool    `json:"speculative_enabled,omitempty"`
	TriggerPrefixes     []string `json:"trigger_prefixes,omitempty"`
	FileTriggersEnabled *bool    `json:"file_triggers_enabled,omitempty"`
}

// UpdateWorkspaceTool describes the update_workspace tool.
func UpdateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "update_workspace",
		Description: "Updates an existing Terraform workspace configuration. To add or update workspace tags, use create_workspace_tags.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace to update",
				},
				"new_name": {
					Type:        "string",
					Description: "Optional new name for the workspace",
				},
				"description": {
					Type:        "string",
					Description: "Optional new description; use an empty string to clear it",
				},
				"terraform_version": {
					Type:        "string",
					Description: "Optional new Terraform version to use (e.g., '1.5.0')",
				},
				"working_directory": {
					Type:        "string",
					Description: "Optional new working directory; use an empty string to clear it",
				},
				"auto_apply": {
					Type:        "boolean",
					Description: "Whether to automatically apply successful plans",
				},
				"execution_mode": {
					Type:        "string",
					Description: "Optional execution mode: local or remote",
					Enum:        enumOf(validExecutionModes...),
				},
				"queue_all_runs": {
					Type:        "boolean",
					Description: "Whether to queue all runs",
				},
				"speculative_enabled": {
					Type:        "boolean",
					Description: "Whether speculative plans are enabled",
				},
				"trigger_prefixes": {
					Type:        "array",
					Description: "Optional trigger prefixes; use an empty array to clear them",
					Items:       &jsonschema.Schema{Type: "string"},
				},
				"file_triggers_enabled": {
					Type:        "boolean",
					Description: "Whether file triggers are enabled",
				},
			},
			PropertyOrder: []string{
				"terraform_org_name", "workspace_name", "new_name", "description",
				"terraform_version", "working_directory", "auto_apply", "execution_mode",
				"queue_all_runs", "speculative_enabled", "trigger_prefixes", "file_triggers_enabled",
			},
			Required:             []string{"terraform_org_name", "workspace_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: workspaceDetailsSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update an existing Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func UpdateWorkspaceFunc(ctx context.Context, request *mcp.CallToolRequest, input UpdateWorkspaceArguments) (*mcp.CallToolResult, *WorkspaceDetails, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	options, err := workspaceUpdateOptions(input)
	if err != nil {
		return nil, nil, err
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Update(ctx, terraformOrgName, workspaceName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("updating workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	details := workspaceToDetails(workspace)
	return nil, &details, nil
}

func workspaceUpdateOptions(input UpdateWorkspaceArguments) (tfe.WorkspaceUpdateOptions, error) {
	options := tfe.WorkspaceUpdateOptions{}
	changed := false

	if input.NewName != "" {
		options.Name = tfe.String(input.NewName)
		changed = true
	}
	if input.Description != nil {
		options.Description = input.Description
		changed = true
	}
	if input.TerraformVersion != "" {
		options.TerraformVersion = tfe.String(input.TerraformVersion)
		changed = true
	}
	if input.WorkingDirectory != nil {
		options.WorkingDirectory = input.WorkingDirectory
		changed = true
	}
	if input.AutoApply != nil {
		options.AutoApply = input.AutoApply
		changed = true
	}
	if input.QueueAllRuns != nil {
		options.QueueAllRuns = input.QueueAllRuns
		changed = true
	}
	if input.SpeculativeEnabled != nil {
		options.SpeculativeEnabled = input.SpeculativeEnabled
		changed = true
	}
	if input.FileTriggersEnabled != nil {
		options.FileTriggersEnabled = input.FileTriggersEnabled
		changed = true
	}

	if input.ExecutionMode != "" {
		switch input.ExecutionMode {
		case executionModeLocal, executionModeRemote:
			options.ExecutionMode = tfe.String(input.ExecutionMode)
			changed = true
		default:
			return tfe.WorkspaceUpdateOptions{}, fmt.Errorf("execution_mode %q must be one of: %s", input.ExecutionMode, strings.Join(validExecutionModes, ", "))
		}
	}

	if input.TriggerPrefixes != nil {
		options.TriggerPrefixes = append([]string{}, input.TriggerPrefixes...)
		changed = true
	}

	if !changed {
		return tfe.WorkspaceUpdateOptions{}, fmt.Errorf("at least one workspace setting must be provided")
	}

	return options, nil
}
