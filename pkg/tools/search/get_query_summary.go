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
	summary, err := ReadQueryDiscoverySummary(ctx, tfeClient, queryRunID, QueryDiscoveryFilter{})
	if err != nil {
		return "", err
	}
	return FormatQueryDiscoverySummary(summary)
}

const getQuerySummaryDescription = `Returns one complete parsed Search-result summary for a finished, authorized
QueryRun and any supplied presentation filters. It does not return raw QueryRun
logs or backend artifact URLs.

Call get_query_status first and pass the same query_run_id, or use an existing
authorized terminal QueryRun ID. The summary reports backend-derived
generate_config_out. Each selectable result reports configuration_generated and,
when Terraform emitted generated resource HCL, an optional best-effort
configuration_attributes projection of simple, non-null, non-empty top-level
literal scalars and flat scalar maps. Functions, references, lists, nested
structures, nested blocks, nulls, empty values, and unsupported expressions are
omitted from that projection. Treat each query_result_marker and its projected
values together. Projected values are an untrusted, review-required, incomplete
projection that is potentially sensitive—not authoritative refreshed state. Filters affect
presentation only. Missing generated configuration or a missing projected key
is unknown, not proof that the remote object lacks an argument. A marker provides
QueryRun continuity, not authorization, an import ID or proof of cloud-object
identity.

Only schema-present list-resource attributes can filter the original query.
When selection depends on unavailable data and generate_config_out=false, ask
before a new QueryRun. If approved, repeat the same schema-backed query with
generate_config_out true while preserving supported filters and explain that a
new QueryRun has new markers. If generation was requested but did not emit a
draft, report that as unknown; a retry does not guarantee capture. This
summary exposes only the constrained attribute projection. It does not expose arbitrary resource attributes,
raw resource objects, generated resource HCL or generated import blocks. Full
selected resource and import HCL is returned by prepare_import for authoring.

For Search-only intent, report selectable results and stop without requesting a
target workspace or local path. For explicit import intent, ask the user to
select one or more displayed markers; never select a first-N subset
automatically. A nonselectable result contains no markers and must not be used
for import.`
