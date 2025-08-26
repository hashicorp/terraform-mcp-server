// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetWorkspaceDetails creates a tool to get detailed information about a specific Terraform workspace.
func GetWorkspaceDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_workspace_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform workspace, including configuration, variables, and current state information.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform workspace"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to get details for"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getWorkspaceDetailsHandler(ctx, request, logger)
		},
	}
}

func getWorkspaceDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get required parameters
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'workspace_name' parameter is required", err)
	}
	workspaceName = strings.TrimSpace(workspaceName)

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", err)
	}

	// Get workspace details
	workspace, err := tfeClient.Workspaces.ReadWithOptions(ctx, terraformOrgName, workspaceName, &tfe.WorkspaceReadOptions{
		Include: []tfe.WSIncludeOpt{
			tfe.WSOrganization,
			tfe.WSProject,
			tfe.WSReadme,
			tfe.WSOutputs,
			tfe.WSCurrentRun,
		},
	})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace details", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Workspace Details for: %s/%s\n\n", terraformOrgName, workspaceName))
	builder.WriteString("---\n\n")
	builder.WriteString("Basic Information:\n")
	builder.WriteString(fmt.Sprintf("- Organization: %s\n", workspace.Organization.Name))
	builder.WriteString(fmt.Sprintf("- Project: %s\n", workspace.Project.Name))
	builder.WriteString(fmt.Sprintf("- Name: %s\n", workspace.Name))
	builder.WriteString(fmt.Sprintf("- Description: %s\n", workspace.Description))
	builder.WriteString(fmt.Sprintf("- Execution Mode: %s\n", workspace.ExecutionMode))
	builder.WriteString(fmt.Sprintf("- Terraform Version: %s\n", workspace.TerraformVersion))
	builder.WriteString(fmt.Sprintf("- Auto Apply: %t\n", workspace.AutoApply))
	builder.WriteString(fmt.Sprintf("- Created At: %s\n", workspace.CreatedAt))
	builder.WriteString(fmt.Sprintf("- Updated At: %s\n", workspace.UpdatedAt))

	if workspace.VCSRepo != nil {
		builder.WriteString("---\n\n")
		builder.WriteString("VCS Configuration:\n")
		builder.WriteString(fmt.Sprintf("- Provider: %s\n", workspace.VCSRepo.ServiceProvider))
		builder.WriteString(fmt.Sprintf("- Repository: %s\n", workspace.VCSRepo.RepositoryHTTPURL))
		builder.WriteString(fmt.Sprintf("- Branch: %s\n", workspace.VCSRepo.Branch))
	}

	// Fetch variables separately since they're not included in the workspace read options
	variables, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{})
	if err != nil {
		logger.WithError(err).Warn("failed to fetch workspace variables")
		variables = &tfe.VariableList{} // Initialize empty list if fetch fails
	}

	builder.WriteString("---\n\n")
	if len(variables.Items) > 0 {
		builder.WriteString("Variables:\n")
		builder.WriteString("| Key | Description | Category | HCL | Sensitive | Value |\n")
		builder.WriteString("|-----|-------------|----------|-----|-----------|-------|\n")
		for _, variable := range variables.Items {
			value := "<sensitive>"
			if !variable.Sensitive {
				value = variable.Value
			}
			description := variable.Description
			if description == "" {
				description = "N/A"
			}
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %s | %t | %t | %s |\n",
				variable.Key,
				strings.ReplaceAll(description, "|", "\\|"),
				string(variable.Category),
				variable.HCL,
				variable.Sensitive,
				strings.ReplaceAll(value, "|", "\\|"),
			))
		}
	} else {
		builder.WriteString("No variables configured.\n")
	}

	if len(workspace.Outputs) > 0 {
		builder.WriteString("---\n\n")
		builder.WriteString("Outputs:\n")
		builder.WriteString("| Name | Type | Sensitive | Value |\n")
		builder.WriteString("|------|------|-----------|-------|\n")
		for _, output := range workspace.Outputs {
			var value interface{} = "<sensitive>"
			if !output.Sensitive {
				value = output.Value
			}
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %t | %s |\n",
				output.Name,
				output.Type,
				output.Sensitive,
				value,
			))
		}
	} else {
		builder.WriteString("No outputs available.\n")
	}

	if workspace.CurrentRun != nil {
		builder.WriteString("---\n\n")
		builder.WriteString("Current Run Details:\n")
		run, err := tfeClient.Runs.Read(ctx, workspace.CurrentRun.ID)
		if err == nil {
			builder.WriteString(fmt.Sprintf("- Message: %s\n", strings.ReplaceAll(run.Message, "|", "\\|")))
			builder.WriteString(fmt.Sprintf("- Source: %s\n", run.Source))
			builder.WriteString(fmt.Sprintf("- Status: %s\n", run.Status))
		}
	} else {
		builder.WriteString("No current run.\n")
	}

	builder.WriteString("---\n\n")
	workspaceReadmeReader, err := tfeClient.Workspaces.Readme(ctx, workspace.ID)
	if err == nil && workspaceReadmeReader != nil {
		readmeBytes, err := io.ReadAll(workspaceReadmeReader)
		if err == nil && len(readmeBytes) > 0 {
			builder.WriteString("Workspace README:\n")
			builder.WriteString(string(readmeBytes))
		}
	}

	return mcp.NewToolResultText(builder.String()), nil
}
