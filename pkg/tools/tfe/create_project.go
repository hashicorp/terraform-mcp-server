// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var validExecutionModes = []string{"local", "agent", "remote"}
var validExecutionModesStr = strings.Join(validExecutionModes, ", ")

// CreateProject creates a tool to create a new Terraform project.
func CreateProject(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_project",
			mcp.WithDescription(`Creates a new Terraform project in the specified organization.`),
			mcp.WithTitleAnnotation("Create a new Terraform project"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription+" to create the project in"),
			),
			mcp.WithString("project_name",
				mcp.Required(),
				mcp.Description("The project name. Must be 3-40 characters and may contain letters, numbers, spaces, hyphens, and underscores. It cannot start or end with a space."),
				mcp.MinLength(3),
				mcp.MaxLength(40),
				mcp.Pattern(`^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$`),
			),
			mcp.WithString("description",
				mcp.Description("Optional project description. Must be no more than 256 characters"),
				mcp.MaxLength(256),
			),
			mcp.WithString("default_execution_mode",
				mcp.Description("Optional default execution mode for workspaces in the project: "+validExecutionModesStr+". If not set, workspaces inherit the organization's default execution mode."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createProjectHandler(ctx, request, logger)
		},
	}
}

func createProjectHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	projectName, err := request.RequireString("project_name")
	if err != nil {
		return ToolError(logger, "missing required input: project_name", err)
	}
	description := GetTrimmedString(request, "description", "")
	defaultExecutionMode := GetTrimmedString(request, "default_execution_mode", "")
	terraformOrgName = strings.TrimSpace(terraformOrgName)
	projectName = strings.TrimSpace(projectName)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	options := tfe.ProjectCreateOptions{
		Name: projectName,
	}

	if description != "" {
		options.Description = &description
	}

	if defaultExecutionMode != "" {
		mode := strings.ToLower(defaultExecutionMode)
		if !slices.Contains(validExecutionModes, mode) {
			return ToolErrorf(logger, "invalid default_execution_mode %q - must be one of: %s", defaultExecutionMode, validExecutionModesStr)
		}
		options.DefaultExecutionMode = tfe.String(mode)
	}

	project, err := tfeClient.Projects.Create(ctx, terraformOrgName, options)
	if err != nil {
		return ToolErrorf(logger, "failed to create project %q in org %q: %v", projectName, terraformOrgName, err)
	}

	projectJSON, err := json.Marshal(&ProjectSummary{project.ID, project.Name})
	if err != nil {
		return ToolError(logger, "failed to marshal created project summary", err)
	}

	return mcp.NewToolResultText(string(projectJSON)), nil
}
