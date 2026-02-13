// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetApplyDetails creates a tool to get detailed apply information and logs for a Terraform run.
func GetApplyDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_apply_details",
			mcp.WithDescription(`Fetches the detailed apply results and logs for a Terraform run, showing what actually happened during the apply phase including any errors. Accepts a run ID, retrieves the associated apply, and returns the apply status, resource change counts, and the full apply log output.`),
			mcp.WithTitleAnnotation("Get detailed apply results and logs for a Terraform run"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to get apply details for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getApplyDetailsHandler(ctx, req, logger)
		},
	}
}

func getApplyDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID, err := request.RequireString("run_id")
	if err != nil {
		return ToolError(logger, "missing required input: run_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// Read the run with the apply relationship included so we get the apply ID
	run, err := tfeClient.Runs.ReadWithOptions(ctx, runID, &tfe.RunReadOptions{
		Include: []tfe.RunIncludeOpt{tfe.RunApply},
	})
	if err != nil {
		return ToolErrorf(logger, "run not found: %s", runID)
	}

	if run.Apply == nil {
		return ToolErrorf(logger, "no apply associated with run: %s (run status: %s)", runID, run.Status)
	}

	apply := run.Apply

	var sb strings.Builder

	// Header and metadata
	sb.WriteString(fmt.Sprintf("# Apply Details for Run %s\n\n", run.ID))
	sb.WriteString(fmt.Sprintf("**Apply ID:** %s\n", apply.ID))
	sb.WriteString(fmt.Sprintf("**Apply Status:** %s\n", apply.Status))
	sb.WriteString(fmt.Sprintf("**Run Status:** %s\n", run.Status))
	sb.WriteString(fmt.Sprintf("**Resource Additions:** %d\n", apply.ResourceAdditions))
	sb.WriteString(fmt.Sprintf("**Resource Changes:** %d\n", apply.ResourceChanges))
	sb.WriteString(fmt.Sprintf("**Resource Destructions:** %d\n", apply.ResourceDestructions))
	sb.WriteString(fmt.Sprintf("**Resource Imports:** %d\n", apply.ResourceImports))

	if apply.StatusTimestamps != nil {
		sb.WriteString("\n## Timestamps\n\n")
		if !apply.StatusTimestamps.QueuedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Queued:** %s\n", apply.StatusTimestamps.QueuedAt.Format("2006-01-02 15:04:05 UTC")))
		}
		if !apply.StatusTimestamps.StartedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Started:** %s\n", apply.StatusTimestamps.StartedAt.Format("2006-01-02 15:04:05 UTC")))
		}
		if !apply.StatusTimestamps.FinishedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Finished:** %s\n", apply.StatusTimestamps.FinishedAt.Format("2006-01-02 15:04:05 UTC")))
		}
		if !apply.StatusTimestamps.ErroredAt.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Errored:** %s\n", apply.StatusTimestamps.ErroredAt.Format("2006-01-02 15:04:05 UTC")))
		}
		if !apply.StatusTimestamps.CanceledAt.IsZero() {
			sb.WriteString(fmt.Sprintf("- **Canceled:** %s\n", apply.StatusTimestamps.CanceledAt.Format("2006-01-02 15:04:05 UTC")))
		}
	}

	// Check if the apply has reached a state where logs are available
	switch apply.Status {
	case "pending", "queued", "unreachable":
		sb.WriteString(fmt.Sprintf("\n> **Note:** Apply logs are not yet available because the apply status is `%s`. ", apply.Status))
		sb.WriteString("Re-run this tool after the apply has started or completed to see the full logs.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Fetch apply logs
	logReader, err := tfeClient.Applies.Logs(ctx, apply.ID)
	if err != nil {
		logger.WithError(err).Warn("Could not fetch apply logs")
		sb.WriteString("\n> **Note:** Could not retrieve apply logs.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		logger.WithError(err).Warn("Could not read apply logs")
		sb.WriteString("\n> **Note:** Could not read apply logs.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	logContent := string(logBytes)

	if logContent == "" {
		sb.WriteString("\n## Apply Logs\n\nNo log output available.\n")
	} else {
		// Truncate very large logs to keep the response reasonable
		const maxLogSize = 50000
		truncated := false
		if len(logContent) > maxLogSize {
			// Keep the last portion of the log (errors are usually at the end)
			logContent = logContent[len(logContent)-maxLogSize:]
			truncated = true
		}

		sb.WriteString("\n## Apply Logs\n\n")
		if truncated {
			sb.WriteString("*(Log output truncated -- showing last portion which typically contains errors)*\n\n")
		}
		sb.WriteString("```\n")
		sb.WriteString(logContent)
		if !strings.HasSuffix(logContent, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
