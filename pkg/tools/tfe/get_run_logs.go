// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetRunLogs creates a tool to fetch logs from a Terraform run (plan and/or apply logs).
func GetRunLogs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_run_logs",
			mcp.WithDescription(`Fetches logs from a Terraform run. You can retrieve plan logs, apply logs, or both.`),
			mcp.WithTitleAnnotation("Get logs from a Terraform run"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to get logs for"),
			),
			mcp.WithString("log_type",
				mcp.Description("Type of logs to retrieve: 'plan', 'apply', or 'both'"),
				mcp.Enum("plan", "apply", "both"),
				mcp.DefaultString("both"),
			),
			mcp.WithBoolean("include_metadata",
				mcp.Description("Include run metadata along with logs"),
				mcp.DefaultBool(true),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunLogsHandler(ctx, req, logger)
		},
	}
}

func getRunLogsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID, err := request.RequireString("run_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'run_id' parameter is required", err)
	}

	logType := request.GetString("log_type", "both")
	includeMetadata := request.GetBool("include_metadata", true)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	// First, fetch the run details to get Plan and Apply IDs
	run, err := tfeClient.Runs.Read(ctx, runID)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading run details", err)
	}

	result := make(map[string]interface{})

	// Add metadata if requested
	if includeMetadata {
		result["run_id"] = run.ID
		result["status"] = string(run.Status)
		result["message"] = run.Message
		result["created_at"] = run.CreatedAt
		result["terraform_version"] = run.TerraformVersion
		result["has_changes"] = run.HasChanges
		result["is_destroy"] = run.IsDestroy
	}

	// Fetch plan logs if requested
	if (logType == "plan" || logType == "both") && run.Plan != nil {
		planLogs, err := fetchLogs(ctx, tfeClient, "plan", run.Plan.ID, logger)
		if err != nil {
			result["plan_logs_error"] = err.Error()
			logger.WithError(err).Warn("Failed to fetch plan logs")
		} else {
			result["plan_logs"] = planLogs
			if includeMetadata {
				result["plan_id"] = run.Plan.ID
				result["plan_status"] = string(run.Plan.Status)
			}
		}
	} else if logType == "plan" && run.Plan == nil {
		result["plan_logs"] = "Plan not yet available for this run"
	}

	// Fetch apply logs if requested
	if (logType == "apply" || logType == "both") && run.Apply != nil {
		applyLogs, err := fetchLogs(ctx, tfeClient, "apply", run.Apply.ID, logger)
		if err != nil {
			result["apply_logs_error"] = err.Error()
			logger.WithError(err).Warn("Failed to fetch apply logs")
		} else {
			result["apply_logs"] = applyLogs
			if includeMetadata {
				result["apply_id"] = run.Apply.ID
				result["apply_status"] = string(run.Apply.Status)
			}
		}
	} else if logType == "apply" && run.Apply == nil {
		result["apply_logs"] = "Apply not yet available for this run (may not have been applied yet)"
	}

	// Check if we got any logs
	if result["plan_logs"] == nil && result["apply_logs"] == nil {
		if logType == "both" {
			result["message"] = "No logs available yet. The run may still be queued or in progress."
		}
	}

	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling run logs result", err)
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// fetchLogs is a helper function to fetch logs from either Plans or Applies
func fetchLogs(ctx context.Context, tfeClient *tfe.Client, logType string, id string, logger *log.Logger) (string, error) {
	var logReader io.Reader
	var err error

	// Fetch logs based on type
	switch logType {
	case "plan":
		logReader, err = tfeClient.Plans.Logs(ctx, id)
	case "apply":
		logReader, err = tfeClient.Applies.Logs(ctx, id)
	default:
		return "", fmt.Errorf("invalid log type: %s", logType)
	}

	if err != nil {
		return "", fmt.Errorf("fetching %s logs: %w", logType, err)
	}

	// Read all logs from the reader
	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return "", fmt.Errorf("reading %s logs: %w", logType, err)
	}

	return string(logBytes), nil
}

