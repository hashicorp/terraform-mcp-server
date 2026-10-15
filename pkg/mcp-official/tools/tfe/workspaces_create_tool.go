// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
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
	TerraformOrgName    string `json:"terraform_org_name"`
	WorkspaceName       string `json:"workspace_name"`
	Description         string `json:"description,omitempty"`
	TerraformVersion    string `json:"terraform_version,omitempty"`
	WorkingDirectory    string `json:"working_directory,omitempty"`
	AutoApply           bool   `json:"auto_apply,omitempty"`
	ExecutionMode       string `json:"execution_mode,omitempty"`
	ProjectID           string `json:"project_id,omitempty"`
	VCSRepoIdentifier   string `json:"vcs_repo_identifier,omitempty"`
	VCSRepoBranch       string `json:"vcs_repo_branch,omitempty"`
	VCSRepoOAuthTokenID string `json:"vcs_repo_oauth_token_id,omitempty"`
	Tags                string `json:"tags,omitempty"`
}

// CreateWorkspaceTool describes the create_workspace tool.
func CreateWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_workspace",
		Description: "Creates a new Terraform workspace in the specified organization.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace to create",
				},
				"description": {
					Type:        "string",
					Description: "Optional description for the workspace",
				},
				"terraform_version": {
					Type:        "string",
					Description: "Optional Terraform version to use (e.g., '1.5.0')",
				},
				"working_directory": {
					Type:        "string",
					Description: "Optional working directory for Terraform operations",
				},
				"auto_apply": {
					Type:        "boolean",
					Description: "Whether to automatically apply successful plans",
					Default:     json.RawMessage("false"),
				},
				"execution_mode": {
					Type:        "string",
					Description: "Optional execution mode: local or remote. When omitted, the workspace inherits the applicable default.",
					Enum:        enumOf(validExecutionModes...),
				},
				"project_id": {
					Type:        "string",
					Description: "Optional project ID to associate the workspace with",
				},
				"vcs_repo_identifier": {
					Type:        "string",
					Description: "Optional VCS repository identifier (e.g., 'org/repo')",
				},
				"vcs_repo_branch": {
					Type:        "string",
					Description: "Optional VCS repository branch (default: main/master)",
				},
				"vcs_repo_oauth_token_id": {
					Type:        "string",
					Description: "OAuth token ID for VCS integration",
				},
				"tags": {
					Type:        "string",
					Description: "Optional comma-separated list of tags to apply to the workspace",
				},
			},
			PropertyOrder: []string{
				"terraform_org_name", "workspace_name", "description", "terraform_version",
				"working_directory", "auto_apply", "execution_mode", "project_id",
				"vcs_repo_identifier", "vcs_repo_branch", "vcs_repo_oauth_token_id", "tags",
			},
			Required:             []string{"terraform_org_name", "workspace_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
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
	options := tfe.WorkspaceCreateOptions{
		Name:       tfe.String(workspaceName),
		AutoApply:  tfe.Bool(input.AutoApply),
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
		switch input.ExecutionMode {
		case executionModeLocal, executionModeRemote:
			options.ExecutionMode = tfe.String(input.ExecutionMode)
		default:
			return tfe.WorkspaceCreateOptions{}, fmt.Errorf("execution_mode %q must be one of: %s", input.ExecutionMode, strings.Join(validExecutionModes, ", "))
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
