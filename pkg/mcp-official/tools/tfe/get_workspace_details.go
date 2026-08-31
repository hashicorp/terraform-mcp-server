// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed static/default-cli-run.md
var defaultWorkspaceReadme string

// GetWorkspaceDetailsArguments holds the input parameters for retrieving workspace details.
type GetWorkspaceDetailsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The name of the workspace to get details for"`
}

// WorkspaceToolResult contains the structured result returned by workspace tools.
type WorkspaceToolResult struct {
	Success     bool             `json:"success"`
	Type        string           `json:"type"`
	WorkspaceID string           `json:"workspace_id"`
	Workspace   WorkspaceDetails `json:"workspace"`
}

// WorkspaceVariable contains the workspace variable fields exposed in tool results.
type WorkspaceVariable struct {
	ID          string           `json:"id"`
	Key         string           `json:"key"`
	Value       string           `json:"value,omitempty"`
	Description string           `json:"description,omitempty"`
	Category    tfe.CategoryType `json:"category"`
	HCL         bool             `json:"hcl"`
	Sensitive   bool             `json:"sensitive"`
}

// WorkspaceDetails contains workspace configuration and status information.
type WorkspaceDetails struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Description         string              `json:"description,omitempty"`
	OrganizationName    string              `json:"organization_name,omitempty"`
	ProjectID           string              `json:"project_id,omitempty"`
	AutoApply           bool                `json:"auto_apply"`
	ExecutionMode       string              `json:"execution_mode"`
	TerraformVersion    string              `json:"terraform_version,omitempty"`
	WorkingDirectory    string              `json:"working_directory,omitempty"`
	QueueAllRuns        bool                `json:"queue_all_runs"`
	SpeculativeEnabled  bool                `json:"speculative_enabled"`
	FileTriggersEnabled bool                `json:"file_triggers_enabled"`
	TriggerPrefixes     []string            `json:"trigger_prefixes,omitempty"`
	Locked              bool                `json:"locked"`
	ResourceCount       int                 `json:"resource_count"`
	Tags                []string            `json:"tags,omitempty"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	Variables           []WorkspaceVariable `json:"variables,omitempty"`
	Readme              string              `json:"readme,omitempty"`
}

func workspaceResult(toolType string, workspace *tfe.Workspace, variables []*tfe.Variable, readme string) *WorkspaceToolResult {
	details := WorkspaceDetails{
		ID: workspace.ID, Name: workspace.Name, Description: workspace.Description,
		AutoApply: workspace.AutoApply, ExecutionMode: workspace.ExecutionMode,
		TerraformVersion: workspace.TerraformVersion, WorkingDirectory: workspace.WorkingDirectory,
		QueueAllRuns: workspace.QueueAllRuns, SpeculativeEnabled: workspace.SpeculativeEnabled,
		FileTriggersEnabled: workspace.FileTriggersEnabled, TriggerPrefixes: workspace.TriggerPrefixes,
		Locked: workspace.Locked, ResourceCount: workspace.ResourceCount,
		Tags: workspace.TagNames, CreatedAt: workspace.CreatedAt, UpdatedAt: workspace.UpdatedAt, Readme: readme,
	}
	if workspace.Organization != nil {
		details.OrganizationName = workspace.Organization.Name
	}
	if workspace.Project != nil {
		details.ProjectID = workspace.Project.ID
	}
	if len(variables) > 0 {
		details.Variables = make([]WorkspaceVariable, len(variables))
		for i, variable := range variables {
			details.Variables[i] = WorkspaceVariable{ID: variable.ID, Key: variable.Key, Value: variable.Value, Description: variable.Description, Category: variable.Category, HCL: variable.HCL, Sensitive: variable.Sensitive}
		}
	}
	return &WorkspaceToolResult{Success: true, Type: toolType, WorkspaceID: workspace.ID, Workspace: details}
}

func GetWorkspaceDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_workspace_details",
		Description: "Fetches detailed information about a specific Terraform workspace, including configuration, variables, and current state information.",
		Annotations: &mcp.ToolAnnotations{Title: "Get detailed information about a Terraform workspace", OpenWorldHint: ptr(true), ReadOnlyHint: true, DestructiveHint: ptr(false)},
	}
}

func GetWorkspaceDetailsFunc(ctx context.Context, _ *mcp.CallToolRequest, input GetWorkspaceDetailsArguments) (*mcp.CallToolResult, *WorkspaceToolResult, error) {
	orgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if orgName == "" || workspaceName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name and workspace_name are required")
	}
	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace %q not found in org %q: %w", workspaceName, orgName, err)
	}

	var workspaceVariables []*tfe.Variable
	variables, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{})
	if err == nil {
		workspaceVariables = variables.Items
	}
	readme := strings.ReplaceAll(defaultWorkspaceReadme, "<<your-terraform-org>>", orgName)
	readme = strings.ReplaceAll(readme, "<<your-terraform-workspace>>", workspace.Name)
	if reader, err := tfeClient.Workspaces.Readme(ctx, workspace.ID); err == nil && reader != nil {
		if content, err := io.ReadAll(reader); err == nil && len(content) > 0 {
			readme = string(content)
		}
	}
	return nil, workspaceResult("get_workspace_details", workspace, workspaceVariables, readme), nil
}
