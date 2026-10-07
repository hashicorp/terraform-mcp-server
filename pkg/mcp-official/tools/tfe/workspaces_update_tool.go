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
	TerraformOrgName    string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceName       string `json:"workspace_name" jsonschema:"The name of the workspace to update"`
	NewName             string `json:"new_name,omitempty" jsonschema:"Optional new name for the workspace"`
	Description         string `json:"description,omitempty" jsonschema:"Optional new description for the workspace"`
	TerraformVersion    string `json:"terraform_version,omitempty" jsonschema:"Optional new Terraform version to use (e.g., '1.5.0')"`
	WorkingDirectory    string `json:"working_directory,omitempty" jsonschema:"Optional new working directory for Terraform operations"`
	AutoApply           string `json:"auto_apply,omitempty" jsonschema:"Whether to automatically apply successful plans: 'true' or 'false'"`
	ExecutionMode       string `json:"execution_mode,omitempty" jsonschema:"Execution mode: 'remote', 'local', or 'agent'"`
	QueueAllRuns        string `json:"queue_all_runs,omitempty" jsonschema:"Whether to queue all runs: 'true' or 'false'"`
	SpeculativeEnabled  string `json:"speculative_enabled,omitempty" jsonschema:"Whether speculative plans are enabled: 'true' or 'false'"`
	TriggerPrefixes     string `json:"trigger_prefixes,omitempty" jsonschema:"Optional comma-separated list of trigger prefixes"`
	FileTriggersEnabled string `json:"file_triggers_enabled,omitempty" jsonschema:"Whether file triggers are enabled: 'true' or 'false'"`
	Tags                string `json:"tags,omitempty" jsonschema:"Accepted for legacy compatibility but ignored; use create_workspace_tags to modify tags"`
}

// UpdateWorkspaceTool describes the update_workspace tool.
func UpdateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "update_workspace",
		Description:  "Updates an existing Terraform workspace configuration.",
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

	if input.NewName != "" {
		options.Name = tfe.String(input.NewName)
	}
	if input.Description != "" {
		options.Description = tfe.String(input.Description)
	}
	if input.TerraformVersion != "" {
		options.TerraformVersion = tfe.String(input.TerraformVersion)
	}
	if input.WorkingDirectory != "" {
		options.WorkingDirectory = tfe.String(input.WorkingDirectory)
	}
	if input.AutoApply != "" {
		options.AutoApply = tfe.Bool(strings.EqualFold(input.AutoApply, "true"))
	}
	if input.QueueAllRuns != "" {
		options.QueueAllRuns = tfe.Bool(strings.EqualFold(input.QueueAllRuns, "true"))
	}
	if input.SpeculativeEnabled != "" {
		options.SpeculativeEnabled = tfe.Bool(strings.EqualFold(input.SpeculativeEnabled, "true"))
	}
	if input.FileTriggersEnabled != "" {
		options.FileTriggersEnabled = tfe.Bool(strings.EqualFold(input.FileTriggersEnabled, "true"))
	}

	if input.ExecutionMode != "" {
		executionMode := strings.ToLower(input.ExecutionMode)
		switch executionMode {
		case "remote", "local", "agent":
			options.ExecutionMode = tfe.String(executionMode)
		default:
			return tfe.WorkspaceUpdateOptions{}, fmt.Errorf("execution_mode %q must be one of: remote, local, agent", input.ExecutionMode)
		}
	}

	if input.TriggerPrefixes != "" {
		prefixes := strings.Split(strings.TrimSpace(input.TriggerPrefixes), ",")
		for i := range prefixes {
			prefixes[i] = strings.TrimSpace(prefixes[i])
		}
		options.TriggerPrefixes = prefixes
	}

	return options, nil
}
