// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const SourceName = "terraform-mcp-server"

// CreateWorkspace creates a tool to create a new Terraform workspace.
func CreateWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_workspace",
			mcp.WithDescription(`Creates a new Terraform workspace in the specified organization. This is a destructive operation that will create new infrastructure resources.`),
			mcp.WithTitleAnnotation("Create a new Terraform workspace"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to create"),
			),
			mcp.WithString("description",
				mcp.Description("Optional description for the workspace"),
			),
			mcp.WithString("terraform_version",
				mcp.Description("Optional Terraform version to use (e.g., '1.5.0')"),
			),
			mcp.WithString("working_directory",
				mcp.Description("Optional working directory for Terraform operations"),
			),
			mcp.WithString("auto_apply",
				mcp.Description("Whether to automatically apply successful plans: 'true' or 'false' (default: 'false')"),
			),
			mcp.WithString("execution_mode",
				mcp.Description("Execution mode: 'remote', 'local', or 'agent' (default: 'remote')"),
			),
			mcp.WithString("project_id",
				mcp.Description("Optional project ID to associate the workspace with"),
			),
			mcp.WithString("vcs_repo_identifier",
				mcp.Description("Optional VCS repository identifier (e.g., 'org/repo')"),
			),
			mcp.WithString("vcs_repo_branch",
				mcp.Description("Optional VCS repository branch (default: main/master)"),
			),
			mcp.WithString("vcs_repo_oauth_token_id",
				mcp.Description("OAuth token ID for VCS integration"),
			),
			mcp.WithString("tags",
				mcp.Description("Optional comma-separated list of tags to apply to the workspace"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createWorkspaceHandler(ctx, request, logger)
		},
	}
}

func createWorkspaceHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	workspaceName = strings.TrimSpace(workspaceName)

	description := request.GetString("description", "")
	terraformVersion := request.GetString("terraform_version", "")
	workingDirectory := request.GetString("working_directory", "")
	autoApplyStr := request.GetString("auto_apply", "false")
	executionModeStr := request.GetString("execution_mode", "")
	projectID := request.GetString("project_id", "")
	vcsRepoIdentifier := request.GetString("vcs_repo_identifier", "")
	vcsRepoBranch := request.GetString("vcs_repo_branch", "")
	vcsRepoOAuthTokenID := request.GetString("vcs_repo_oauth_token_id", "")
	tagsStr := request.GetString("tags", "")

	autoApply := strings.ToLower(autoApplyStr) == "true"

	executionMode := "remote"
	switch strings.ToLower(executionModeStr) {
	case "local":
		executionMode = "local"
	case "agent":
		executionMode = "agent"
	case "remote", "":
		executionMode = "remote"
	default:
		return ToolErrorf(logger, "invalid execution_mode '%s' - must be 'remote', 'local', or 'agent'", executionModeStr)
	}

	var tags []*tfe.Tag
	if tagsStr != "" {
		tagNames := strings.Split(strings.TrimSpace(tagsStr), ",")
		tags = make([]*tfe.Tag, 0, len(tagNames))
		for _, tagName := range tagNames {
			tagName = strings.TrimSpace(tagName)
			if tagName != "" {
				tags = append(tags, &tfe.Tag{Name: tagName})
			}
		}
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	options := &tfe.WorkspaceCreateOptions{
		Name:       &workspaceName,
		AutoApply:  &autoApply,
		Tags:       tags,
		SourceName: tfe.String(SourceName),
	}

	if description != "" {
		options.Description = &description
	}
	if terraformVersion != "" {
		options.TerraformVersion = &terraformVersion
	}
	if workingDirectory != "" {
		options.WorkingDirectory = &workingDirectory
	}
	if projectID != "" {
		options.Project = &tfe.Project{ID: projectID}
	}

	if executionModeStr != "" {
		switch executionMode {
		case "local":
			options.ExecutionMode = tfe.String("local")
		case "agent":
			options.ExecutionMode = tfe.String("agent")
		case "remote":
			options.ExecutionMode = tfe.String("remote")
		}
	}

	if vcsRepoIdentifier != "" {
		if vcsRepoOAuthTokenID == "" {
			return ToolError(logger, "vcs_repo_oauth_token_id is required when vcs_repo_identifier is provided", nil)
		}

		vcsRepo := &tfe.VCSRepoOptions{
			Identifier:   &vcsRepoIdentifier,
			OAuthTokenID: &vcsRepoOAuthTokenID,
		}

		if vcsRepoBranch != "" {
			vcsRepo.Branch = &vcsRepoBranch
		}

		options.VCSRepo = vcsRepo
	}

	workspace, err := tfeClient.Workspaces.Create(ctx, terraformOrgName, *options)
	if err != nil {
		return ToolErrorf(logger, "failed to create workspace '%s' in org '%s': %v", workspaceName, terraformOrgName, err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "create_workspace", tfeClient, workspace, logger)
	if err != nil {
		return ToolError(logger, "failed to get workspace details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
