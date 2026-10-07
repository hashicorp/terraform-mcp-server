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

const workspaceSourceName = "terraform-mcp-server"

// CreateWorkspaceArguments holds the inputs for creating a workspace.
type CreateWorkspaceArguments struct {
	TerraformOrgName    string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceName       string `json:"workspace_name" jsonschema:"The name of the workspace to create"`
	Description         string `json:"description,omitempty" jsonschema:"Optional description for the workspace"`
	TerraformVersion    string `json:"terraform_version,omitempty" jsonschema:"Optional Terraform version to use (e.g., '1.5.0')"`
	WorkingDirectory    string `json:"working_directory,omitempty" jsonschema:"Optional working directory for Terraform operations"`
	AutoApply           string `json:"auto_apply,omitempty" jsonschema:"Whether to automatically apply successful plans: 'true' or 'false' (default: 'false')"`
	ExecutionMode       string `json:"execution_mode,omitempty" jsonschema:"Execution mode: 'remote', 'local', or 'agent' (default: 'remote')"`
	ProjectID           string `json:"project_id,omitempty" jsonschema:"Optional project ID to associate the workspace with"`
	VCSRepoIdentifier   string `json:"vcs_repo_identifier,omitempty" jsonschema:"Optional VCS repository identifier (e.g., 'org/repo')"`
	VCSRepoBranch       string `json:"vcs_repo_branch,omitempty" jsonschema:"Optional VCS repository branch (default: main/master)"`
	VCSRepoOAuthTokenID string `json:"vcs_repo_oauth_token_id,omitempty" jsonschema:"OAuth token ID for VCS integration"`
	Tags                string `json:"tags,omitempty" jsonschema:"Optional comma-separated list of tags to apply to the workspace"`
}

// CreateWorkspaceTool describes the create_workspace tool.
func CreateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "create_workspace",
		Description:  "Creates a new Terraform workspace in the specified organization.",
		OutputSchema: workspaceDetailsSchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateWorkspaceFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateWorkspaceArguments) (*mcp.CallToolResult, *WorkspaceDetails, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	options, err := workspaceCreateOptions(input, workspaceName)
	if err != nil {
		return nil, nil, err
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Create(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("creating workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	details := workspaceToDetails(workspace)
	return nil, &details, nil
}

func workspaceCreateOptions(input CreateWorkspaceArguments, workspaceName string) (tfe.WorkspaceCreateOptions, error) {
	autoApply := strings.EqualFold(input.AutoApply, "true")
	options := tfe.WorkspaceCreateOptions{
		Name:       tfe.String(workspaceName),
		AutoApply:  tfe.Bool(autoApply),
		SourceName: tfe.String(workspaceSourceName),
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
	if input.ProjectID != "" {
		options.Project = &tfe.Project{ID: input.ProjectID}
	}

	if input.ExecutionMode != "" {
		executionMode := strings.ToLower(input.ExecutionMode)
		switch executionMode {
		case "remote", "local", "agent":
			options.ExecutionMode = tfe.String(executionMode)
		default:
			return tfe.WorkspaceCreateOptions{}, fmt.Errorf("execution_mode %q must be one of: remote, local, agent", input.ExecutionMode)
		}
	}

	if input.VCSRepoIdentifier != "" {
		if input.VCSRepoOAuthTokenID == "" {
			return tfe.WorkspaceCreateOptions{}, fmt.Errorf("vcs_repo_oauth_token_id is required when vcs_repo_identifier is provided")
		}

		options.VCSRepo = &tfe.VCSRepoOptions{
			Identifier:   tfe.String(input.VCSRepoIdentifier),
			OAuthTokenID: tfe.String(input.VCSRepoOAuthTokenID),
		}
		if input.VCSRepoBranch != "" {
			options.VCSRepo.Branch = tfe.String(input.VCSRepoBranch)
		}
	}

	if input.Tags != "" {
		for _, name := range strings.Split(strings.TrimSpace(input.Tags), ",") {
			if name := strings.TrimSpace(name); name != "" {
				options.Tags = append(options.Tags, &tfe.Tag{Name: name})
			}
		}
	}

	return options, nil
}
