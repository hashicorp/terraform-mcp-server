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

// GetApplyLogs creates a tool to retrieve the logs of a specific Terraform apply.
func GetApplyLogs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_apply_logs",
			mcp.WithDescription(`Retrieves the logs of a specific Terraform apply.`),
			mcp.WithTitleAnnotation("Get logs for a Terraform apply"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("apply_id",
				mcp.Required(),
				mcp.Description("The ID of the apply to get logs for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getApplyLogsHandler(ctx, req, logger)
		},
	}
}

func getApplyLogsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	applyID, err := request.RequireString("apply_id")
	if err != nil {
		return ToolError(logger, "missing required input: apply_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	logReader, err := tfeClient.Applies.Logs(ctx, applyID)
	if err != nil {
		return ToolErrorf(logger, "failed to retrieve apply logs: %s", applyID)
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return ToolError(logger, "failed to read apply logs", err)
	}

	return mcp.NewToolResultText(string(logBytes)), nil
}
