// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

const (
	queryStatusPollInterval = 2 * time.Second
	queryStatusWaitBudget   = 5 * time.Second
	queryStatusMaxReads     = 3
)

type queryStatusResponse struct {
	ID                string                         `json:"query_run_id"`
	Status            tfe.QueryRunStatus             `json:"status"`
	Terminal          bool                           `json:"terminal"`
	RetryAfterSeconds int                            `json:"retry_after_seconds,omitempty"`
	Message           string                         `json:"message"`
	StatusTimestamps  *queryStatusTimestampsResponse `json:"status_timestamps,omitempty"`
}

type queryStatusTimestampsResponse struct {
	CanceledAt      *time.Time `json:"canceled_at,omitempty"`
	ErroredAt       *time.Time `json:"errored_at,omitempty"`
	ForceCanceledAt *time.Time `json:"force_canceled_at,omitempty"`
	QueuingAt       *time.Time `json:"queuing_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	RunningAt       *time.Time `json:"running_at,omitempty"`
}

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
			mcp.WithOutputSchema[queryStatusResponse](),
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

	pollCtx, cancel := context.WithTimeout(ctx, queryStatusWaitBudget)
	defer cancel()

	response, err := waitForQueryStatus(pollCtx, tfeClient, strings.TrimSpace(queryRunID), queryStatusPollInterval, queryStatusMaxReads)
	if err != nil {
		return getQueryStatusToolErrorf(logger, "failed to get query run %q: %v", queryRunID, err)
	}

	text, err := json.Marshal(response)
	if err != nil {
		return getQueryStatusToolErrorf(logger, "failed to marshal query run %q: %v", queryRunID, err)
	}
	return mcp.NewToolResultStructured(response, fmt.Sprintf("%s\n\n%s", response.Message, text)), nil
}

func readQueryStatus(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
	if err != nil {
		return "", err
	}
	return marshalQueryStatus(queryRun)
}

func waitForQueryStatus(ctx context.Context, tfeClient *tfe.Client, queryRunID string, pollInterval time.Duration, maxReads int) (*queryStatusResponse, error) {
	var lastResponse *queryStatusResponse
	for reads := range maxReads {
		queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
		if err != nil {
			if lastResponse != nil && ctx.Err() == context.DeadlineExceeded {
				return lastResponse, nil
			}
			return nil, err
		}
		response := queryStatusResponseFromRun(queryRun)
		if isTerminalQueryStatus(queryRun.Status) {
			response.Message = fmt.Sprintf("Query run reached terminal status %q. Call get_query_summary with the same query_run_id to retrieve the result summary.", queryRun.Status)
			return &response, nil
		}
		response.RetryAfterSeconds = int(pollInterval / time.Second)
		response.Message = fmt.Sprintf("Query run is still %q. Call get_query_status again with the same query_run_id after %d seconds.", queryRun.Status, response.RetryAfterSeconds)
		lastResponse = &response
		if reads == maxReads-1 {
			return lastResponse, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastResponse, nil
		case <-timer.C:
		}
	}
	return lastResponse, nil
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
	response, err := json.Marshal(queryStatusResponseFromRun(queryRun))
	if err != nil {
		return "", fmt.Errorf("marshaling query run: %w", err)
	}
	return string(response), nil
}

func queryStatusResponseFromRun(queryRun *tfe.QueryRun) queryStatusResponse {
	return queryStatusResponse{
		ID:               queryRun.ID,
		Status:           queryRun.Status,
		Terminal:         isTerminalQueryStatus(queryRun.Status),
		StatusTimestamps: queryStatusTimestamps(queryRun.StatusTimestamps),
	}
}

func queryStatusTimestamps(timestamps *tfe.QueryRunStatusTimestamps) *queryStatusTimestampsResponse {
	if timestamps == nil {
		return nil
	}
	response := &queryStatusTimestampsResponse{}
	if !timestamps.CanceledAt.IsZero() {
		response.CanceledAt = &timestamps.CanceledAt
	}
	if !timestamps.ErroredAt.IsZero() {
		response.ErroredAt = &timestamps.ErroredAt
	}
	if !timestamps.ForceCanceledAt.IsZero() {
		response.ForceCanceledAt = &timestamps.ForceCanceledAt
	}
	if !timestamps.QueuingAt.IsZero() {
		response.QueuingAt = &timestamps.QueuingAt
	}
	if !timestamps.FinishedAt.IsZero() {
		response.FinishedAt = &timestamps.FinishedAt
	}
	if !timestamps.RunningAt.IsZero() {
		response.RunningAt = &timestamps.RunningAt
	}
	return response
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
execute_query response. This tool checks every two seconds while status is pending,
queued, or running and returns once status is finished, errored, or canceled, or after at
most three status checks or five seconds. If the
run is still active, the tool returns its current status and instructs you to call this
tool again with the same query_run_id after retry_after_seconds. After a terminal status
is returned, call get_query_summary with the same ID. Do not use curl or call the HCP
Terraform query API directly.`
