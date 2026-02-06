// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetPlanDetails creates a tool to get detailed information about a specific Terraform plan.
func GetPlanDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_plan_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform plan.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform plan"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("plan_id",
				mcp.Required(),
				mcp.Description("The ID of the plan to get details for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanDetailsHandler(ctx, req, logger)
		},
	}
}

func getPlanDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	planID, err := request.RequireString("plan_id")
	if err != nil {
		return ToolError(logger, "missing required input: plan_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	plan, err := tfeClient.Plans.Read(ctx, planID)
	if err != nil {
		return ToolErrorf(logger, "plan not found: %s", planID)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, plan)
	if err != nil {
		return ToolError(logger, "failed to marshal plan details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
