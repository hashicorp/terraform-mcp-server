// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// CreateRunSafe creates a tool to create a new Terraform run without destructive options.
func CreateRunSafe(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_run",
			mcp.WithDescription(`Creates a new Terraform run in the specified workspace.`),
			mcp.WithTitleAnnotation("Create a new Terraform run"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to create a run in"),
			),
			mcp.WithString("run_type",
				mcp.Description("A run type for the run"),
				mcp.Enum("plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply"),
				mcp.DefaultString("plan_and_apply"),
			),
			mcp.WithString("message",
				mcp.Description("Optional message for the run"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createRunSafeHandler(ctx, req, logger)
		},
	}
}

func createRunSafeHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	runType := request.GetString("run_type", "plan_and_apply")
	message := request.GetString("message", "Triggered via Terraform MCP Server")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace", err)
	}

	options := &tfe.RunCreateOptions{
		Workspace: workspace,
	}
	switch runType {
	case "plan_and_apply":
		options.AutoApply = tfe.Bool(false)
	case "refresh_state":
		options.RefreshOnly = tfe.Bool(true)
	case "plan_only":
		options.PlanOnly = tfe.Bool(true)
	case "allow_empty_apply":
		options.AllowEmptyApply = tfe.Bool(true)
	}

	if message != "" {
		options.Message = &message
	}

	run, err := tfeClient.Runs.Create(ctx, *options)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "creating run", err)
	}

	var buf bytes.Buffer
	if err := jsonapi.MarshalPayload(&buf, run); err != nil {
		return nil, utils.LogAndReturnError(logger, "marshaling run response", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(buf.String()),
		},
	}, nil
}

// CreateRun creates a tool to create a new Terraform run.
func CreateRun(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_run",
			mcp.WithDescription(`Creates a new Terraform run in the specified workspace.`),
			mcp.WithTitleAnnotation("Create a new Terraform run"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to create a run in"),
			),
			mcp.WithString("run_type",
				mcp.Description("A run type for the run"),
				mcp.Enum("plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply", "auto_approve", "is_destroy"),
				mcp.DefaultString("plan_and_apply"),
			),
			mcp.WithString("message",
				mcp.Description("Optional message for the run"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createRunHandler(ctx, req, logger)
		},
	}
}

func createRunHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	runType := request.GetString("run_type", "plan_and_apply")
	message := request.GetString("message", "Triggered via Terraform MCP Server")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace", err)
	}

	options := &tfe.RunCreateOptions{
		Workspace: workspace,
	}
	switch runType {
	case "plan_and_apply":
		options.AutoApply = tfe.Bool(false)
	case "refresh_state":
		options.RefreshOnly = tfe.Bool(true)
	case "plan_only":
		options.PlanOnly = tfe.Bool(true)
	case "allow_empty_apply":
		options.AllowEmptyApply = tfe.Bool(true)
	case "auto_approve":
		options.AutoApply = tfe.Bool(true)
	case "is_destroy":
		options.IsDestroy = tfe.Bool(true)
	}

	if message != "" {
		options.Message = &message
	}

	run, err := tfeClient.Runs.Create(ctx, *options)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "creating run", err)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, run)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling run creation", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
