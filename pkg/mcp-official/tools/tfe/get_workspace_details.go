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

// WorkspaceActions contains actions currently available for a workspace.
type WorkspaceActions struct {
	IsDestroyable bool `json:"is_destroyable"`
}

// WorkspacePermissions contains the caller's permissions for a workspace.
type WorkspacePermissions struct {
	CanDestroy           bool  `json:"can_destroy"`
	CanForceDelete       *bool `json:"can_force_delete"`
	CanForceUnlock       bool  `json:"can_force_unlock"`
	CanLock              bool  `json:"can_lock"`
	CanManageHYOK        bool  `json:"can_manage_hyok"`
	CanManageRunTasks    bool  `json:"can_manage_run_tasks"`
	CanQueueApply        bool  `json:"can_queue_apply"`
	CanQueueDestroy      bool  `json:"can_queue_destroy"`
	CanQueueRun          bool  `json:"can_queue_run"`
	CanReadSettings      bool  `json:"can_read_settings"`
	CanReadStateVersions bool  `json:"can_read_state_versions"`
	CanReadVariable      bool  `json:"can_read_variable"`
	CanUnlock            bool  `json:"can_unlock"`
	CanUpdate            bool  `json:"can_update"`
	CanUpdateVariable    bool  `json:"can_update_variable"`
}

// WorkspaceSettingOverwrites identifies settings overridden at the workspace level.
type WorkspaceSettingOverwrites struct {
	AgentPool     *bool `json:"agent_pool"`
	ExecutionMode *bool `json:"execution_mode"`
}

// WorkspaceVCSRepo contains the workspace's VCS repository configuration.
type WorkspaceVCSRepo struct {
	Branch            string `json:"branch"`
	DisplayIdentifier string `json:"display_identifier"`
	Identifier        string `json:"identifier"`
	IngressSubmodules bool   `json:"ingress_submodules"`
	OAuthTokenID      string `json:"oauth_token_id"`
	GHAInstallationID string `json:"github_app_installation_id"`
	RepositoryHTTPURL string `json:"repository_http_url"`
	ServiceProvider   string `json:"service_provider"`
	Tags              bool   `json:"tags"`
	TagsRegex         string `json:"tags_regex"`
	WebhookURL        string `json:"webhook_url"`
	SourceDirectory   string `json:"source_directory"`
	TagPrefix         string `json:"tag_prefix"`
}

// WorkspaceDetails contains workspace configuration and status information.
type WorkspaceDetails struct {
	ID                          string                      `json:"id"`
	Actions                     *WorkspaceActions           `json:"actions"`
	AllowDestroyPlan            bool                        `json:"allow_destroy_plan"`
	AssessmentsEnabled          bool                        `json:"assessments_enabled"`
	AutoApply                   bool                        `json:"auto_apply"`
	AutoApplyRunTrigger         bool                        `json:"auto_apply_run_trigger"`
	AutoDestroyAt               *time.Time                  `json:"auto_destroy_at,omitempty"`
	AutoDestroyActivityDuration *string                     `json:"auto_destroy_activity_duration,omitempty"`
	CanQueueDestroyPlan         bool                        `json:"can_queue_destroy_plan"`
	CreatedAt                   time.Time                   `json:"created_at"`
	Description                 string                      `json:"description"`
	Environment                 string                      `json:"environment"`
	ExecutionMode               string                      `json:"execution_mode"`
	FileTriggersEnabled         bool                        `json:"file_triggers_enabled"`
	GlobalRemoteState           bool                        `json:"global_remote_state"`
	ProjectRemoteState          bool                        `json:"project_remote_state"`
	InheritsProjectAutoDestroy  bool                        `json:"inherits_project_auto_destroy"`
	HYOKEnabled                 *bool                       `json:"hyok_enabled"`
	Locked                      bool                        `json:"locked"`
	MigrationEnvironment        string                      `json:"migration_environment"`
	Name                        string                      `json:"name"`
	NoCodeUpgradeAvailable      bool                        `json:"no_code_upgrade_available"`
	Operations                  bool                        `json:"operations"`
	Permissions                 *WorkspacePermissions       `json:"permissions"`
	QueueAllRuns                bool                        `json:"queue_all_runs"`
	SpeculativeEnabled          bool                        `json:"speculative_enabled"`
	Source                      string                      `json:"source"`
	SourceName                  string                      `json:"source_name"`
	SourceURL                   string                      `json:"source_url"`
	SourceModuleID              string                      `json:"source_module_id"`
	StructuredRunOutputEnabled  bool                        `json:"structured_run_output_enabled"`
	TerraformVersion            string                      `json:"terraform_version"`
	TriggerPrefixes             []string                    `json:"trigger_prefixes"`
	TriggerPatterns             []string                    `json:"trigger_patterns"`
	VCSRepo                     *WorkspaceVCSRepo           `json:"vcs_repo"`
	WorkingDirectory            string                      `json:"working_directory"`
	UpdatedAt                   time.Time                   `json:"updated_at"`
	ResourceCount               int                         `json:"resource_count"`
	ApplyDurationAverage        time.Duration               `json:"apply_duration_average"`
	PlanDurationAverage         time.Duration               `json:"plan_duration_average"`
	PolicyCheckFailures         int                         `json:"policy_check_failures"`
	RunFailures                 int                         `json:"run_failures"`
	RunsCount                   int                         `json:"workspace_kpis_runs_count"`
	TagNames                    []string                    `json:"tag_names"`
	SettingOverwrites           *WorkspaceSettingOverwrites `json:"setting_overwrites"`
	OrganizationName            string                      `json:"organization_name,omitempty"`
	ProjectID                   string                      `json:"project_id,omitempty"`
	Variables                   []WorkspaceVariable         `json:"variables,omitempty"`
	Readme                      string                      `json:"readme,omitempty"`
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

func workspaceResult(toolType string, workspace *tfe.Workspace, variables []*tfe.Variable, readme string) *WorkspaceToolResult {
	details := WorkspaceDetails{
		ID:                         workspace.ID,
		AllowDestroyPlan:           workspace.AllowDestroyPlan,
		AssessmentsEnabled:         workspace.AssessmentsEnabled,
		AutoApply:                  workspace.AutoApply,
		AutoApplyRunTrigger:        workspace.AutoApplyRunTrigger,
		CanQueueDestroyPlan:        workspace.CanQueueDestroyPlan,
		CreatedAt:                  workspace.CreatedAt,
		Description:                workspace.Description,
		Environment:                workspace.Environment,
		ExecutionMode:              workspace.ExecutionMode,
		FileTriggersEnabled:        workspace.FileTriggersEnabled,
		GlobalRemoteState:          workspace.GlobalRemoteState,
		ProjectRemoteState:         workspace.ProjectRemoteState,
		InheritsProjectAutoDestroy: workspace.InheritsProjectAutoDestroy,
		HYOKEnabled:                workspace.HYOKEnabled,
		Locked:                     workspace.Locked,
		MigrationEnvironment:       workspace.MigrationEnvironment,
		Name:                       workspace.Name,
		NoCodeUpgradeAvailable:     workspace.NoCodeUpgradeAvailable,
		Operations:                 workspace.Operations,
		QueueAllRuns:               workspace.QueueAllRuns,
		SpeculativeEnabled:         workspace.SpeculativeEnabled,
		Source:                     string(workspace.Source),
		SourceName:                 workspace.SourceName,
		SourceURL:                  workspace.SourceURL,
		SourceModuleID:             workspace.SourceModuleID,
		StructuredRunOutputEnabled: workspace.StructuredRunOutputEnabled,
		TerraformVersion:           workspace.TerraformVersion,
		TriggerPrefixes:            workspace.TriggerPrefixes,
		TriggerPatterns:            workspace.TriggerPatterns,
		WorkingDirectory:           workspace.WorkingDirectory,
		UpdatedAt:                  workspace.UpdatedAt,
		ResourceCount:              workspace.ResourceCount,
		ApplyDurationAverage:       workspace.ApplyDurationAverage,
		PlanDurationAverage:        workspace.PlanDurationAverage,
		PolicyCheckFailures:        workspace.PolicyCheckFailures,
		RunFailures:                workspace.RunFailures,
		RunsCount:                  workspace.RunsCount,
		TagNames:                   workspace.TagNames,
		Readme:                     readme,
	}
	if value, err := workspace.AutoDestroyAt.Get(); err == nil {
		details.AutoDestroyAt = &value
	}
	if value, err := workspace.AutoDestroyActivityDuration.Get(); err == nil {
		details.AutoDestroyActivityDuration = &value
	}
	if workspace.Actions != nil {
		details.Actions = &WorkspaceActions{IsDestroyable: workspace.Actions.IsDestroyable}
	}
	if workspace.Permissions != nil {
		details.Permissions = &WorkspacePermissions{
			CanDestroy:           workspace.Permissions.CanDestroy,
			CanForceDelete:       workspace.Permissions.CanForceDelete,
			CanForceUnlock:       workspace.Permissions.CanForceUnlock,
			CanLock:              workspace.Permissions.CanLock,
			CanManageHYOK:        workspace.Permissions.CanManageHYOK,
			CanManageRunTasks:    workspace.Permissions.CanManageRunTasks,
			CanQueueApply:        workspace.Permissions.CanQueueApply,
			CanQueueDestroy:      workspace.Permissions.CanQueueDestroy,
			CanQueueRun:          workspace.Permissions.CanQueueRun,
			CanReadSettings:      workspace.Permissions.CanReadSettings,
			CanReadStateVersions: workspace.Permissions.CanReadStateVersions,
			CanReadVariable:      workspace.Permissions.CanReadVariable,
			CanUnlock:            workspace.Permissions.CanUnlock,
			CanUpdate:            workspace.Permissions.CanUpdate,
			CanUpdateVariable:    workspace.Permissions.CanUpdateVariable,
		}
	}
	if workspace.SettingOverwrites != nil {
		details.SettingOverwrites = &WorkspaceSettingOverwrites{
			AgentPool: workspace.SettingOverwrites.AgentPool, ExecutionMode: workspace.SettingOverwrites.ExecutionMode,
		}
	}
	if workspace.VCSRepo != nil {
		details.VCSRepo = &WorkspaceVCSRepo{
			Branch: workspace.VCSRepo.Branch, DisplayIdentifier: workspace.VCSRepo.DisplayIdentifier,
			Identifier: workspace.VCSRepo.Identifier, IngressSubmodules: workspace.VCSRepo.IngressSubmodules,
			OAuthTokenID: workspace.VCSRepo.OAuthTokenID, GHAInstallationID: workspace.VCSRepo.GHAInstallationID,
			RepositoryHTTPURL: workspace.VCSRepo.RepositoryHTTPURL, ServiceProvider: workspace.VCSRepo.ServiceProvider,
			Tags: workspace.VCSRepo.Tags, TagsRegex: workspace.VCSRepo.TagsRegex, WebhookURL: workspace.VCSRepo.WebhookURL,
			SourceDirectory: workspace.VCSRepo.SourceDirectory, TagPrefix: workspace.VCSRepo.TagPrefix,
		}
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
