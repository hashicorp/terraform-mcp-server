// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ActionRun creates a tool to apply, discard or cancel a Terraform run
func ActionRun(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("action_run",
			mcp.WithDescription(`Performs a variety of actions on a Terraform run. It can be used to approve and apply, discard or cancel a run.`),
			mcp.WithTitleAnnotation("Apply, Discard or Cancel a Terraform run"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("run_action",
				mcp.Required(),
				mcp.Description("The action to perform on the run (e.g., 'apply', 'discard', 'cancel')"),
				mcp.Enum("apply", "discard", "cancel"),
			),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to perform the action on"),
			),

			mcp.WithString("comment",
				mcp.Description("Optional comment for the action"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return actionRunHandler(ctx, req, logger)
		},
	}
}

func actionRunHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runAction, err := request.RequireString("run_action")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'run_action' parameter is required", err)
	}

	runID, err := request.RequireString("run_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'run_id' parameter is required", err)
	}

	comment := request.GetString("comment", "Triggered via Terraform MCP Server")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	var msg string
	switch runAction {
	case "apply":
		err = tfeClient.Runs.Apply(ctx, runID, tfe.RunApplyOptions{Comment: &comment})
		msg = "Run approved and applied successfully, run the `get_run_details` tool to get more information about the run."
	case "discard":
		err = tfeClient.Runs.Discard(ctx, runID, tfe.RunDiscardOptions{Comment: &comment})
		msg = "Run discarded successfully"
	case "cancel":
		err = tfeClient.Runs.Cancel(ctx, runID, tfe.RunCancelOptions{Comment: &comment})
		msg = "Run canceled successfully"
	default:
		return nil, utils.LogAndReturnError(logger, "invalid run action", err)
	}

	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("executing %s action on run %s", runAction, runID), err)
	}

	result := map[string]interface{}{
		"success": true,
		"message": msg,
		"run_id":  runID,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling action run result", err)
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}
