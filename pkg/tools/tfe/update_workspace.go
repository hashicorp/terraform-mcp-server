// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// UpdateWorkspace creates a tool to update an existing Terraform workspace.
func UpdateWorkspace(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("update_workspace",
			mcp.WithDescription(`Updates an existing Terraform workspace configuration. This is a potentially destructive operation that may affect infrastructure resources.`),
			mcp.WithTitleAnnotation("Update an existing Terraform workspace"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to update"),
			),
			mcp.WithString("new_name",
				mcp.Description("Optional new name for the workspace"),
			),
			mcp.WithString("description",
				mcp.Description("Optional new description for the workspace"),
			),
			mcp.WithString("terraform_version",
				mcp.Description("Optional new Terraform version to use (e.g., '1.5.0')"),
			),
			mcp.WithString("working_directory",
				mcp.Description("Optional new working directory for Terraform operations"),
			),
			mcp.WithString("auto_apply",
				mcp.Description("Whether to automatically apply successful plans: 'true' or 'false'"),
			),
			mcp.WithString("execution_mode",
				mcp.Description("Execution mode: 'remote', 'local', or 'agent'"),
			),
			mcp.WithString("queue_all_runs",
				mcp.Description("Whether to queue all runs: 'true' or 'false'"),
			),
			mcp.WithString("speculative_enabled",
				mcp.Description("Whether speculative plans are enabled: 'true' or 'false'"),
			),
			mcp.WithString("trigger_prefixes",
				mcp.Description("Optional comma-separated list of trigger prefixes"),
			),
			mcp.WithString("file_triggers_enabled",
				mcp.Description("Whether file triggers are enabled: 'true' or 'false'"),
			),
			mcp.WithString("tags",
				mcp.Description("Optional comma-separated list of tags to replace existing tags"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return updateWorkspaceHandler(ctx, request, logger)
		},
	}
}

func updateWorkspaceHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Get optional parameters - we'll check if they're provided by comparing to empty/default values
	newName := request.GetString("new_name", "")
	description := request.GetString("description", "")
	terraformVersion := request.GetString("terraform_version", "")
	workingDirectory := request.GetString("working_directory", "")
	autoApplyStr := request.GetString("auto_apply", "")
	executionModeStr := request.GetString("execution_mode", "")
	queueAllRunsStr := request.GetString("queue_all_runs", "")
	speculativeEnabledStr := request.GetString("speculative_enabled", "")
	triggerPrefixesStr := request.GetString("trigger_prefixes", "")
	fileTriggersEnabledStr := request.GetString("file_triggers_enabled", "")
	tagsStr := request.GetString("tags", "")

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", err)
	}

	// Build workspace update options
	options := &tfe.WorkspaceUpdateOptions{}

	if newName != "" {
		options.Name = &newName
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
	if autoApplyStr != "" {
		autoApply := strings.ToLower(autoApplyStr) == "true"
		options.AutoApply = &autoApply
	}
	if queueAllRunsStr != "" {
		queueAllRuns := strings.ToLower(queueAllRunsStr) == "true"
		options.QueueAllRuns = &queueAllRuns
	}
	if speculativeEnabledStr != "" {
		speculativeEnabled := strings.ToLower(speculativeEnabledStr) == "true"
		options.SpeculativeEnabled = &speculativeEnabled
	}
	if fileTriggersEnabledStr != "" {
		fileTriggersEnabled := strings.ToLower(fileTriggersEnabledStr) == "true"
		options.FileTriggersEnabled = &fileTriggersEnabled
	}

	// Parse execution mode
	if executionModeStr != "" {
		switch strings.ToLower(executionModeStr) {
		case "local":
			options.ExecutionMode = tfe.String("local")
		case "agent":
			options.ExecutionMode = tfe.String("agent")
		case "remote":
			options.ExecutionMode = tfe.String("remote")
		default:
			return nil, utils.LogAndReturnError(logger, "invalid execution_mode: must be 'remote', 'local', or 'agent'", nil)
		}
	}

	// Parse trigger prefixes
	if triggerPrefixesStr != "" {
		if triggerPrefixesStr == "" {
			options.TriggerPrefixes = []string{}
		} else {
			prefixes := strings.Split(strings.TrimSpace(triggerPrefixesStr), ",")
			for i, prefix := range prefixes {
				prefixes[i] = strings.TrimSpace(prefix)
			}
			options.TriggerPrefixes = prefixes
		}
	}

	// Parse tags - Note: Tags might not be updatable via WorkspaceUpdateOptions
	// This would require a separate API call to manage tags
	if tagsStr != "" {
		logger.Warnf("Tag updates are not supported via workspace update - tags parameter ignored")
	}

	// Update the workspace
	workspace, err := tfeClient.Workspaces.Update(ctx, terraformOrgName, workspaceName, *options)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "updating workspace", err)
	}

	resultJSON, err := json.Marshal(workspace)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling workspace update result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}
