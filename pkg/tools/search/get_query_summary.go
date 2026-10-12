// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

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
				mcp.Description("Finished QueryRun ID from execute_query or an existing authorized QueryRun."),
			),
			mcp.WithString("resource_type", mcp.Description("Optional exact Search list-resource type filter.")),
			mcp.WithString("address", mcp.Description("Optional exact list block address filter.")),
			mcp.WithString("name_contains", mcp.Description("Optional case-insensitive display-name substring.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (default 100, maximum 200)."), mcp.Min(1), mcp.Max(200)),
			mcp.WithString("after", mcp.Description("Previous next_cursor to continue the same QueryRun snapshot and filters.")),
			mcp.WithOutputSchema[QueryDiscoveryPage](),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getQuerySummaryHandler(ctx, request, logger)
		},
	}
}

func getQuerySummaryHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	queryRunID, err := request.RequireString("query_run_id")
	if err != nil || strings.TrimSpace(queryRunID) == "" {
		return toolErrorf(logger, "get_query_summary", "missing required input: query_run_id")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "failed to get Terraform client; check session configuration")
	}
	filter := QueryDiscoveryFilter{ResourceType: request.GetString("resource_type", ""), Address: request.GetString("address", ""), NameContains: request.GetString("name_contains", ""), Limit: request.GetInt("limit", 100), After: request.GetString("after", "")}
	return getQuerySummaryWithClient(ctx, tfeClient, queryRunID, filter, logger)
}

func getQuerySummaryWithClient(ctx context.Context, tfeClient *tfe.Client, queryRunID string, filter QueryDiscoveryFilter, logger *log.Logger) (*mcp.CallToolResult, error) {
	queryRunID = strings.TrimSpace(queryRunID)
	if queryRunID == "" {
		return toolErrorf(logger, "get_query_summary", "missing required input: query_run_id")
	}
	summary, err := ReadQueryDiscoveryPage(ctx, tfeClient, queryRunID, filter)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "query summary unavailable: %v", err)
	}
	text, err := FormatQueryDiscoveryPage(summary)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "query summary unavailable: %v", err)
	}
	return mcp.NewToolResultStructured(summary, text), nil
}

// ReadQuerySummary retains the plain-Go first-page entry point for callers
// that used it before V1. It now returns only bounded, provenance-checked
// Search results, not raw log rows or backend diagnostic detail.
func ReadQuerySummary(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	page, err := ReadQueryDiscoveryPage(ctx, tfeClient, strings.TrimSpace(queryRunID), QueryDiscoveryFilter{})
	if err != nil {
		return "", err
	}
	return FormatQueryDiscoveryPage(page)
}

const getQuerySummaryDescription = `Returns bounded, backend-scoped Search results from a finished QueryRun.

Call get_query_status first. Pass the same query_run_id; an existing authorized
QueryRun ID can also be used without rerunning Search. A selectable response
includes source workspace provenance and lists[].results[] with per-result
query_result_marker, identity and identity_version. Results are not selected
until the user chooses them. Follow next_cursor as after with unchanged filters
to browse the same snapshot. A nonselectable response has no result markers;
do not attempt import preparation. A marker is selection continuity, not an
import ID, authorization or proof of the correct cloud object.`
