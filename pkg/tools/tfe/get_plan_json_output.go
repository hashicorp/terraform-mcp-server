// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetPlanJSONOutput creates a tool to retrieve the structured JSON output of a specific Terraform plan.
func GetPlanJSONOutput(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_plan_json_output",
			mcp.WithDescription(`Retrieves the structured JSON output of a specific Terraform plan. This includes detailed information about resource changes (create, update, delete), attribute values before and after, and plan metadata. This is more structured and easier to parse than plain logs.`),
			mcp.WithTitleAnnotation("Get JSON output for a Terraform plan"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("plan_id",
				mcp.Required(),
				mcp.Description("The ID of the plan to get JSON output for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanJSONOutputHandler(ctx, req, logger)
		},
	}
}

func getPlanJSONOutputHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	planID, err := request.RequireString("plan_id")
	if err != nil {
		return ToolError(logger, "missing required input: plan_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	jsonBytes, err := tfeClient.Plans.ReadJSONOutput(ctx, planID)
	if err != nil {
		return ToolErrorf(logger, "failed to retrieve plan JSON output: %s", planID)
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}
