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

// ListRuns creates a tool to list Terraform runs in a workspace.
func ListRuns(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_runs",
			mcp.WithDescription(`List or search Terraform runs in a specific workspace with optional filtering.`),
			mcp.WithTitleAnnotation("List Terraform runs"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("Lists the runs in Terraform Cloud/Enterprise organization based on filters if no workspace is specified"),
			),
			mcp.WithString("workspace_name",
				mcp.Description("If specified, lists the runs in the given workspace instead of the organization based on filters"),
			),
			mcp.WithString("vcs_username",
				mcp.Description("Searches for runs that match the VCS username you supply"),
			),
			mcp.WithArray("status",
				mcp.Description("Optional run status filter"),
				mcp.WithStringEnumItems([]string{
					"pending",
					"fetching",
					"fetching_completed",
					"pre_plan_running",
					"pre_plan_completed",
					"queuing",
					"plan_queued",
					"planning",
					"planned",
					"cost_estimating",
					"cost_estimated",
					"policy_checking",
					"policy_override",
					"policy_soft_failed",
					"policy_checked",
					"confirmed",
					"post_plan_running",
					"post_plan_completed",
					"planned_and_finished",
					"planned_and_saved",
					"apply_queued",
					"applying",
					"applied",
					"discarded",
					"errored",
					"canceled",
					"force_canceled",
				},
				),
			),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listRunsHandler(ctx, req, logger)
		},
	}
}

func listRunsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	workspaceName := request.GetString("workspace_name", "")
	vcsUsername := request.GetString("vcs_username", "")
	status := request.GetString("status", "")

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	buf := bytes.NewBuffer(nil)
	if workspaceName != "" {

		// Set up pagination options
		options := &tfe.RunListOptions{
			ListOptions: tfe.ListOptions{
				PageNumber: pagination.Page,
				PageSize:   pagination.PageSize,
			},
		}

		// Set up filtering options
		if status != "" {
			options.Status = status
		}

		if vcsUsername != "" {
			options.User = vcsUsername
		}

		// Get runs for the specified workspace with options
		workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "reading workspace", err)
		}

		runs, err := tfeClient.Runs.List(ctx, workspace.ID, options)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "listing runs in workspace", err)
		}

		err = jsonapi.MarshalPayloadWithoutIncluded(buf, runs)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "marshalling search runs", err)
		}

	} else {

		// Set up pagination options
		options := &tfe.RunListForOrganizationOptions{
			ListOptions: tfe.ListOptions{
				PageNumber: pagination.Page,
				PageSize:   pagination.PageSize,
			},
		}

		// Set up filtering options
		if status != "" {
			options.Status = status
		}

		if vcsUsername != "" {
			options.User = vcsUsername
		}

		// Get runs for the specified organization with options
		runs, err := tfeClient.Runs.ListForOrganization(ctx, terraformOrgName, options)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "listing runs in organization", err)
		}

		err = jsonapi.MarshalPayloadWithoutIncluded(buf, runs)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "marshalling search organization runs", err)
		}
	}

	return mcp.NewToolResultText(buf.String()), nil
}
