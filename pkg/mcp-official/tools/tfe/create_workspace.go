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

const workspaceSourceName = "terraform-mcp-server"

// CreateWorkspaceArguments holds the input parameters for creating a workspace.
type CreateWorkspaceArguments struct {
	TerraformOrgName    string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName       string `json:"workspace_name" jsonschema:"The name of the workspace to create"`
	Description         string `json:"description,omitempty" jsonschema:"Optional description for the workspace"`
	TerraformVersion    string `json:"terraform_version,omitempty" jsonschema:"Optional Terraform version to use (for example, 1.5.0)"`
	WorkingDirectory    string `json:"working_directory,omitempty" jsonschema:"Optional working directory for Terraform operations"`
	AutoApply           bool  `json:"auto_apply,omitempty" jsonschema:"Whether to automatically apply successful plans (default: false)"`
	ExecutionMode       string `json:"execution_mode,omitempty" jsonschema:"Execution mode: remote, local, or agent (default: remote)"`
	ProjectID           string `json:"project_id,omitempty" jsonschema:"Optional project ID to associate the workspace with"`
	VCSRepoIdentifier   string `json:"vcs_repo_identifier,omitempty" jsonschema:"Optional VCS repository identifier (for example, org/repo)"`
	VCSRepoBranch       string `json:"vcs_repo_branch,omitempty" jsonschema:"Optional VCS repository branch"`
	VCSRepoOAuthTokenID string `json:"vcs_repo_oauth_token_id,omitempty" jsonschema:"OAuth token ID for VCS integration"`
	Tags                string `json:"tags,omitempty" jsonschema:"Optional comma-separated list of tags to apply to the workspace"`
}

// WorkspaceMutationResult contains the identifying information and outcome of a workspace mutation.
type WorkspaceMutationResult struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Message       string `json:"message"`
}

func CreateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_workspace",
		Description: "Creates a new Terraform workspace in the specified organization. This operation may result in new infrastructure resources being created by later runs.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform workspace",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func CreateWorkspaceFunc(ctx context.Context, _ *mcp.CallToolRequest, input CreateWorkspaceArguments) (*mcp.CallToolResult, *WorkspaceMutationResult, error) {
	orgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if orgName == "" || workspaceName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name and workspace_name are required")
	}

	executionMode, err := parseExecutionMode(input.ExecutionMode, true)
	if err != nil {
		return nil, nil, err
	}
	options := tfe.WorkspaceCreateOptions{
		Name:       &workspaceName,
		AutoApply:  ptr(input.AutoApply),
		SourceName: tfe.String(workspaceSourceName),
		Tags:       parseWorkspaceTags(input.Tags),
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
	if input.ProjectID != "" {
		options.Project = &tfe.Project{ID: input.ProjectID}
	}
	if input.ExecutionMode != "" {
		options.ExecutionMode = &executionMode
	}
	if input.VCSRepoIdentifier != "" {
		if input.VCSRepoOAuthTokenID == "" {
			return nil, nil, fmt.Errorf("vcs_repo_oauth_token_id is required when vcs_repo_identifier is provided")
		}
		options.VCSRepo = &tfe.VCSRepoOptions{
			Identifier:   &input.VCSRepoIdentifier,
			OAuthTokenID: &input.VCSRepoOAuthTokenID,
		}
		if input.VCSRepoBranch != "" {
			options.VCSRepo.Branch = &input.VCSRepoBranch
		}
	}

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tfeClient.Workspaces.Create(ctx, orgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workspace %q in org %q: %w", workspaceName, orgName, err)
	}
	return nil, &WorkspaceMutationResult{
		WorkspaceID:   workspace.ID,
		WorkspaceName: workspace.Name,
		Message:       "Workspace created successfully",
	}, nil
}

func parseExecutionMode(value string, allowEmpty bool) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" && allowEmpty {
		return "remote", nil
	}
	switch mode {
	case "remote", "local", "agent":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid execution_mode %q; must be remote, local, or agent", value)
	}
}

func parseWorkspaceTags(value string) []*tfe.Tag {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	names := strings.Split(strings.TrimSpace(value), ",")
	tags := make([]*tfe.Tag, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			tags = append(tags, &tfe.Tag{Name: name})
		}
	}
	return tags
}
