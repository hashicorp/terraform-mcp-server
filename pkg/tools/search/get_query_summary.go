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
			mcp.WithOutputSchema[QueryDiscoverySummary](),
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
	filter := QueryDiscoveryFilter{ResourceType: request.GetString("resource_type", ""), Address: request.GetString("address", ""), NameContains: request.GetString("name_contains", "")}
	return getQuerySummaryWithClient(ctx, tfeClient, queryRunID, filter, logger)
}

func getQuerySummaryWithClient(ctx context.Context, tfeClient *tfe.Client, queryRunID string, filter QueryDiscoveryFilter, logger *log.Logger) (*mcp.CallToolResult, error) {
	queryRunID = strings.TrimSpace(queryRunID)
	if queryRunID == "" {
		return toolErrorf(logger, "get_query_summary", "missing required input: query_run_id")
	}
	summary, err := ReadQueryDiscoverySummary(ctx, tfeClient, queryRunID, filter)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "query summary unavailable: %v", err)
	}
	text, err := FormatQueryDiscoverySummary(summary)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "query summary unavailable: %v", err)
	}
	return mcp.NewToolResultStructured(summary, text), nil
}

// ReadQuerySummary retains the plain-Go entry point for callers that used it
// before V1. It now returns only bounded, provenance-checked
// Search results, not raw log rows or backend diagnostic detail.
func ReadQuerySummary(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	summary, err := ReadQueryDiscoverySummary(ctx, tfeClient, strings.TrimSpace(queryRunID), QueryDiscoveryFilter{})
	if err != nil {
		return "", err
	}
	return FormatQueryDiscoverySummary(summary)
}

const getQuerySummaryDescription = `Returns bounded, backend-scoped Search results from a finished QueryRun.

Call get_query_status first, then use the same query_run_id; an existing
authorized QueryRun ID can also be used. Selectable lists[].results[] include
query_result_marker, identity, identity_version, attributes_captured and any
scalar tags from the captured resource object. Keep the marker, display_name
and any tags for each result shown to the user, including in script output.
There is no tag filter; choose from the returned rows with the user. Missing
tags do not prove a resource is untagged. If attributes_captured is false and
tags matter, ask before a new query (for example with generate_config_out true).
It has new markers and may still lack tags. The response contains every result
matching any supplied summary filters. A nonselectable result is returned only
when the validated QueryRun evidence does not support selection. A marker is
continuity, not an import ID,
authorization or proof of object identity.`
