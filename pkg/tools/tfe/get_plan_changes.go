// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/planchanges"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetPlanChanges creates a tool that lists the resources and outputs a Terraform plan changes.
func GetPlanChanges(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_plan_changes",
			mcp.WithDescription(`Lists the resources and outputs a Terraform plan changes: action (delete, replace, forget, update, create, import, move) and address, plus counts per action and resources that drifted outside Terraform. Never returns attribute values. Reads the redacted JSON plan, so unlike get_plan_json_output it does not need workspace admin access. Pass the run ID or the plan ID.`),
			mcp.WithTitleAnnotation("Get the changes in a Terraform plan"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Description("The ID of the run whose plan to read (run-...). Provide this or plan_id."),
			),
			mcp.WithString("plan_id",
				mcp.Description("The ID of the plan to read (plan-...). Provide this or run_id."),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanChangesHandler(ctx, req, logger)
		},
	}
}

func getPlanChangesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID := request.GetString("run_id", "")
	planID := request.GetString("plan_id", "")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	raw, err := planchanges.Fetch(ctx, tfeClient, runID, planID)
	if err != nil {
		return ToolError(logger, "failed to retrieve plan changes", err)
	}

	summary, err := planchanges.Summarize(raw)
	if err != nil {
		return ToolError(logger, "failed to summarize plan changes", err)
	}

	out, err := json.Marshal(summary)
	if err != nil {
		return ToolError(logger, "failed to marshal plan changes", err)
	}
	return mcp.NewToolResultText(string(out)), nil
}
