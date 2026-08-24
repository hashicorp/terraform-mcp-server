// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

type querySummary struct {
	ResourcesDiscovered int                   `json:"resources_discovered"`
	ListCompletions     []queryListCompletion `json:"list_completions"`
}

type queryListCompletion struct {
	Address      string `json:"address"`
	ResourceType string `json:"resource_type"`
	Total        int    `json:"total"`
}

type queryLogRecord struct {
	Type         string               `json:"type"`
	ListComplete *queryListCompletion `json:"list_complete"`
}

// GetQuerySummary retrieves a completed query run's log and summarizes its results.
func GetQuerySummary(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_query_summary",
			mcp.WithDescription(getQuerySummaryDescription),
			mcp.WithTitleAnnotation("Get HCP Terraform query summary"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query_run_id",
				mcp.Required(),
				mcp.Description("Query run ID previously passed to get_query_status."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getQuerySummaryHandler(ctx, request, logger)
		},
	}
}

func getQuerySummaryHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	queryRunID, err := request.RequireString("query_run_id")
	if err != nil || strings.TrimSpace(queryRunID) == "" {
		return getQuerySummaryToolErrorf(logger, "missing required input: query_run_id")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return getQuerySummaryToolErrorf(logger, "failed to get Terraform client: %v", err)
	}

	summary, err := readQuerySummary(ctx, tfeClient, strings.TrimSpace(queryRunID))
	if err != nil {
		return getQuerySummaryToolErrorf(logger, "failed to get query summary for %q: %v", queryRunID, err)
	}

	return mcp.NewToolResultText(summary), nil
}

func readQuerySummary(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	logReader, err := tfeClient.QueryRuns.Logs(ctx, queryRunID)
	if err != nil {
		return "", err
	}

	summary, err := parseQuerySummary(logReader)
	if err != nil {
		return "", err
	}

	response, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("marshaling query summary: %w", err)
	}
	return string(response), nil
}

func parseQuerySummary(reader io.Reader) (*querySummary, error) {
	summary := &querySummary{ListCompletions: []queryListCompletion{}}
	lines := bufio.NewReader(reader)

	for {
		line, err := lines.ReadBytes('\n')
		if len(line) > 0 {
			var record queryLogRecord
			if json.Unmarshal(line, &record) == nil && record.Type == "list_complete" && record.ListComplete != nil {
				summary.ResourcesDiscovered += record.ListComplete.Total
				summary.ListCompletions = append(summary.ListCompletions, *record.ListComplete)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading query log: %w", err)
		}
	}

	return summary, nil
}

func getQuerySummaryToolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("get_query_summary: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}

const getQuerySummaryDescription = `Retrieves and parses the NDJSON log for an HCP Terraform query run.

Call get_query_status first and wait for it to return a terminal status, then pass the
same query_run_id to this tool. The result contains resources_discovered and one
list_completions entry per list block, with its address, resource_type, and total.`
