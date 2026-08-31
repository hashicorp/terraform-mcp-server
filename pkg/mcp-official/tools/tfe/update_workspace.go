// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UpdateWorkspaceArguments holds the input parameters for updating a workspace.
type UpdateWorkspaceArguments struct {
	TerraformOrgName    string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName       string `json:"workspace_name" jsonschema:"The name of the workspace to update"`
	NewName             string `json:"new_name,omitempty" jsonschema:"Optional new name for the workspace"`
	Description         string `json:"description,omitempty" jsonschema:"Optional new description for the workspace"`
	TerraformVersion    string `json:"terraform_version,omitempty" jsonschema:"Optional new Terraform version to use"`
	WorkingDirectory    string `json:"working_directory,omitempty" jsonschema:"Optional new working directory for Terraform operations"`
	AutoApply           string `json:"auto_apply,omitempty" jsonschema:"Whether to automatically apply successful plans: true or false"`
	ExecutionMode       string `json:"execution_mode,omitempty" jsonschema:"Execution mode: remote, local, or agent"`
	QueueAllRuns        string `json:"queue_all_runs,omitempty" jsonschema:"Whether to queue all runs: true or false"`
	SpeculativeEnabled  string `json:"speculative_enabled,omitempty" jsonschema:"Whether speculative plans are enabled: true or false"`
	TriggerPrefixes     string `json:"trigger_prefixes,omitempty" jsonschema:"Optional comma-separated list of trigger prefixes"`
	FileTriggersEnabled string `json:"file_triggers_enabled,omitempty" jsonschema:"Whether file triggers are enabled: true or false"`
	Tags                string `json:"tags,omitempty" jsonschema:"Optional comma-separated list of tags; tag updates are not supported by this operation"`
}

func UpdateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "update_workspace",
		Description: "Updates an existing Terraform workspace configuration. This operation may affect how future infrastructure runs execute.",
		Annotations: &mcp.ToolAnnotations{Title: "Update an existing Terraform workspace", OpenWorldHint: ptr(true), ReadOnlyHint: false, DestructiveHint: ptr(false)},
	}
}

func UpdateWorkspaceFunc(ctx context.Context, _ *mcp.CallToolRequest, input UpdateWorkspaceArguments) (*mcp.CallToolResult, *WorkspaceToolResult, error) {
	orgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if orgName == "" || workspaceName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name and workspace_name are required")
	}
	options := tfe.WorkspaceUpdateOptions{}
	if input.NewName != "" {
		options.Name = &input.NewName
	}
	if input.Description != "" {
		options.Description = &input.Description
	}
	if input.TerraformVersion != "" {
		options.TerraformVersion = &input.TerraformVersion
	}
	if input.WorkingDirectory != "" {
		options.WorkingDirectory = &input.WorkingDirectory
	}
	if input.AutoApply != "" {
		value := strings.EqualFold(input.AutoApply, "true")
		options.AutoApply = &value
	}
	if input.QueueAllRuns != "" {
		value := strings.EqualFold(input.QueueAllRuns, "true")
		options.QueueAllRuns = &value
	}
	if input.SpeculativeEnabled != "" {
		value := strings.EqualFold(input.SpeculativeEnabled, "true")
		options.SpeculativeEnabled = &value
	}
	if input.FileTriggersEnabled != "" {
		value := strings.EqualFold(input.FileTriggersEnabled, "true")
		options.FileTriggersEnabled = &value
	}
	if input.ExecutionMode != "" {
		mode, err := parseExecutionMode(input.ExecutionMode, false)
		if err != nil {
			return nil, nil, err
		}
		options.ExecutionMode = &mode
	}
	if input.TriggerPrefixes != "" {
		options.TriggerPrefixes = parseTriggerPrefixes(input.TriggerPrefixes)
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tfeClient.Workspaces.Update(ctx, orgName, workspaceName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to update workspace %q in org %q: %w", workspaceName, orgName, err)
	}
	return nil, workspaceResult("update_workspace", workspace, nil, ""), nil
}

func parseTriggerPrefixes(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
