// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:embed static/default-cli-run.md
var defaultReadme string

// GetWorkspaceDetails creates a tool to get detailed information about a specific Terraform workspace.
func GetWorkspaceDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_workspace_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform workspace, including configuration, variables, and current state information.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform workspace"),
			mcp.WithOpenWorldHintAnnotation(true),
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

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace details", err)
	}

	buf, err := getWorkspaceDetailsForTools(ctx, "get_workspace_details", tfeClient, workspace, logger, true)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting workspace details for tools", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func getWorkspaceDetailsForTools(ctx context.Context, toolType string, tfeClient *tfe.Client, workspace *tfe.Workspace, logger *log.Logger, opts ...interface{}) (*bytes.Buffer, error) {
	includeDetails := false
	// Check if detailed information is requested
	for _, opt := range opts {
		if includeOpt, ok := opt.(bool); ok && includeOpt {
			includeDetails = true
			break
		}
	}

	result := &client.WorkspaceToolResponse{
		Success:   true,
		Type:      toolType,
		Workspace: workspace,
	}

	if includeDetails {
		// Fetch variables separately since they're not included in the workspace read options
		variables, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{})
		if err != nil {
			logger.WithError(err).Warn("failed to fetch workspace variables")
			variables = &tfe.VariableList{} // Initialize empty list if fetch fails
		}

		readme := defaultReadme
		readme = strings.ReplaceAll(readme, "<<your-terraform-org>>", workspace.Organization.Name)
		readme = strings.ReplaceAll(readme, "<<your-terraform-workspace>>", workspace.Name)

		workspaceReadmeReader, err := tfeClient.Workspaces.Readme(ctx, workspace.ID)
		if err == nil && workspaceReadmeReader != nil {
			readmeBytes, err := io.ReadAll(workspaceReadmeReader)
			if err == nil && len(readmeBytes) > 0 {
				readme = string(readmeBytes)
			}
		}

		result = &client.WorkspaceToolResponse{
			Success:   true,
			Type:      toolType,
			Workspace: workspace,
			Variables: variables.Items,
			Readme:    readme,
		}
	}

	buf := bytes.NewBuffer(nil)
	err := jsonapi.MarshalPayload(buf, result)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling workspace creation result", err)
	}

	return buf, nil
}
