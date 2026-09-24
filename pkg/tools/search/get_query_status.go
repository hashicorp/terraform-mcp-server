// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"errors"
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
	queryStatusPollInterval      = 3 * time.Second
	queryStatusWaitBudget        = 40 * time.Second
	queryStatusRetryAfterSeconds = 5
)

var errQueryStatusWaitBudgetExceeded = errors.New("query status wait budget exceeded")

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

type queryStatusPollConfig struct {
	pollInterval time.Duration
	waitBudget   time.Duration
}

func defaultQueryStatusPollConfig() queryStatusPollConfig {
	return queryStatusPollConfig{
		pollInterval: queryStatusPollInterval,
		waitBudget:   queryStatusWaitBudget,
	}
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
	return getQueryStatusHandlerWithConfig(ctx, request, logger, defaultQueryStatusPollConfig())
}

func getQueryStatusHandlerWithConfig(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, config queryStatusPollConfig) (*mcp.CallToolResult, error) {
	queryRunID, err := request.RequireString("query_run_id")
	queryRunID = strings.TrimSpace(queryRunID)
	if err != nil || queryRunID == "" {
		return getQueryStatusToolErrorf(logger, "missing required input: query_run_id")
	}
	if config.pollInterval <= 0 {
		return getQueryStatusToolErrorf(logger, "poll interval must be greater than zero")
	}
	if config.waitBudget <= 0 {
		return getQueryStatusToolErrorf(logger, "wait budget must be greater than zero")
	}

	pollCtx, cancel := context.WithTimeoutCause(ctx, config.waitBudget, errQueryStatusWaitBudgetExceeded)
	defer cancel()

	tfeClient, err := client.GetTfeClientFromContext(pollCtx, logger)
	if err != nil {
		return getQueryStatusToolErrorf(logger, "failed to get Terraform client: %v", err)
	}
	if err := callerCancellation(ctx); err != nil {
		return getQueryStatusToolErrorf(logger, "failed to get query run %q: %v", queryRunID, err)
	}

	response, err := waitForQueryStatus(ctx, pollCtx, tfeClient, queryRunID, config.pollInterval, logger)
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

func waitForQueryStatus(parentCtx, pollCtx context.Context, tfeClient *tfe.Client, queryRunID string, pollInterval time.Duration, logger *log.Logger) (*queryStatusResponse, error) {
	if pollInterval <= 0 {
		return nil, fmt.Errorf("poll interval must be greater than zero")
	}

	startedAt := time.Now()
	reads := 0
	var lastResponse *queryStatusResponse
	for {
		if err := callerCancellation(parentCtx); err != nil {
			logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "caller_canceled", err)
			return nil, err
		}
		reads++
		queryRun, err := tfeClient.QueryRuns.Read(pollCtx, queryRunID)
		if callerErr := callerCancellation(parentCtx); callerErr != nil {
			logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "caller_canceled", callerErr)
			return nil, callerErr
		}
		if err != nil {
			if errors.Is(context.Cause(pollCtx), errQueryStatusWaitBudgetExceeded) &&
				lastResponse != nil && errors.Is(err, pollCtx.Err()) {
				logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "internal_budget", nil)
				return lastResponse, nil
			}
			logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "read_error", err)
			return nil, err
		}
		response := queryStatusResponseFromRun(queryRun)
		if isTerminalQueryStatus(queryRun.Status) {
			response.Message = fmt.Sprintf("Query run reached terminal status %q. Call get_query_summary with the same query_run_id to retrieve the result summary.", queryRun.Status)
			logQueryStatusPoll(logger, queryRunID, reads, startedAt, &response, "terminal", nil)
			return &response, nil
		}
		response.RetryAfterSeconds = queryStatusRetryAfterSeconds
		response.Message = fmt.Sprintf("Query run is still %q. Call get_query_status again with the same query_run_id after %d seconds.", queryRun.Status, response.RetryAfterSeconds)
		lastResponse = &response
		logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "status_read", nil)

		timer := time.NewTimer(pollInterval)
		select {
		case <-pollCtx.Done():
			timer.Stop()
			if err := callerCancellation(parentCtx); err != nil {
				logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "caller_canceled", err)
				return nil, err
			}
			if errors.Is(context.Cause(pollCtx), errQueryStatusWaitBudgetExceeded) {
				logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "internal_budget", nil)
				return lastResponse, nil
			}
			logQueryStatusPoll(logger, queryRunID, reads, startedAt, lastResponse, "caller_canceled", context.Cause(pollCtx))
			return nil, context.Cause(pollCtx)
		case <-timer.C:
		}
	}
}

func callerCancellation(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return context.Cause(ctx)
}

func logQueryStatusPoll(logger *log.Logger, queryRunID string, pollCount int, startedAt time.Time, response *queryStatusResponse, reason string, err error) {
	if logger == nil {
		return
	}
	fields := log.Fields{
		"query_run_id": queryRunID,
		"poll_count":   pollCount,
		"elapsed":      time.Since(startedAt),
		"reason":       reason,
	}
	if response != nil {
		fields["last_status"] = response.Status
	}
	if err != nil {
		fields["error"] = err
	}
	logger.WithFields(fields).Debug("Polled HCP Terraform query run status")
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
execute_query response. This tool checks every three seconds while status is pending,
queued, or running and returns once status is finished, errored, or canceled. One call
waits for at most 40 seconds. If the run is still active, the tool returns its current
status and instructs you to call this tool again with the same query_run_id after
retry_after_seconds. retry_after_seconds is omitted for a terminal status. After a
terminal status is returned, call get_query_summary with the same ID. Do not use curl or
call the HCP Terraform query API directly.`
