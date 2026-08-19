// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

const (
	queryStatusPollInterval = 3 * time.Second
	queryStatusTimeout      = 2 * time.Minute
)

// GetQueryStatus gets the current status and details of an HCP Terraform query run.
func GetQueryStatus(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_query_status",
			mcp.WithDescription(getQueryStatusDescription),
			mcp.WithTitleAnnotation("Get HCP Terraform query run status"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query_run_id",
				mcp.Required(),
				mcp.Description("Query run ID returned in the execute_query response as data.relationships.latest-query-run.data.id."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getQueryStatusHandler(ctx, request, logger)
		},
	}
}

func getQueryStatusHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	queryRunID, err := request.RequireString("query_run_id")
	if err != nil || strings.TrimSpace(queryRunID) == "" {
		return getQueryStatusToolErrorf(logger, "missing required input: query_run_id")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return getQueryStatusToolErrorf(logger, "failed to get Terraform client: %v", err)
	}

	pollCtx, cancel := context.WithTimeout(ctx, queryStatusTimeout)
	defer cancel()

	response, status, err := waitForQueryStatus(pollCtx, tfeClient, strings.TrimSpace(queryRunID), queryStatusPollInterval)
	if err != nil {
		return getQueryStatusToolErrorf(logger, "failed to get query run %q: %v", queryRunID, err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Query run reached terminal status %q. Do not call get_query_status again for this query run.\n\n%s",
		status,
		response,
	)), nil
}

func readQueryStatus(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
	if err != nil {
		return "", err
	}
	return marshalQueryStatus(queryRun)
}

func waitForQueryStatus(ctx context.Context, tfeClient *tfe.Client, queryRunID string, pollInterval time.Duration) (string, tfe.QueryRunStatus, error) {
	for {
		queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
		if err != nil {
			return "", "", err
		}
		if isTerminalQueryStatus(queryRun.Status) {
			response, err := marshalQueryStatus(queryRun)
			return response, queryRun.Status, err
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", "", fmt.Errorf("timed out waiting for a terminal status: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func isTerminalQueryStatus(status tfe.QueryRunStatus) bool {
	switch status {
	case tfe.QueryRunCanceled, tfe.QueryRunErrored, tfe.QueryRunFinished:
		return true
	default:
		return false
	}
}

func marshalQueryStatus(queryRun *tfe.QueryRun) (string, error) {
	var response bytes.Buffer
	if err := jsonapi.MarshalPayloadWithoutIncluded(&response, queryRun); err != nil {
		return "", fmt.Errorf("marshaling query run: %w", err)
	}
	return response.String(), nil
}

func getQueryStatusToolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("get_query_status: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}

const getQueryStatusDescription = `Fetches an HCP Terraform query run using go-tfe.

Pass the query run ID from data.relationships.latest-query-run.data.id in the
execute_query response. This tool checks every three seconds while status is pending,
queued, or running and returns once status is finished, errored, or canceled. Make one
call only; do not repeatedly call this tool with the same query run ID. The wait times
out after two minutes. Do not use curl or call the HCP Terraform query API directly.`
