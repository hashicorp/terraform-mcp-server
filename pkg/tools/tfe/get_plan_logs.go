// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"io"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetPlanLogs creates a tool to retrieve the logs of a specific Terraform plan.
func GetPlanLogs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_plan_logs",
			mcp.WithDescription(`Retrieves the logs of a specific Terraform plan.`),
			mcp.WithTitleAnnotation("Get logs for a Terraform plan"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("plan_id",
				mcp.Required(),
				mcp.Description("The ID of the plan to get logs for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanLogsHandler(ctx, req, logger)
		},
	}
}

func getPlanLogsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	planID, err := request.RequireString("plan_id")
	if err != nil {
		return ToolError(logger, "missing required input: plan_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	logReader, err := tfeClient.Plans.Logs(ctx, planID)
	if err != nil {
		return ToolErrorf(logger, "failed to retrieve plan logs: %s", planID)
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return ToolError(logger, "failed to read plan logs", err)
	}

	return mcp.NewToolResultText(string(logBytes)), nil
}
