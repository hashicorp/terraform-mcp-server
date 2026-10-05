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
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools/search/importworkflow"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

type querySummary struct {
	ResourcesDiscovered int                   `json:"resources_discovered"`
	Resources           []queryResource       `json:"resources"`
	ListCompletions     []queryListCompletion `json:"list_completions"`
	Diagnostics         []queryDiagnostic     `json:"diagnostics"`

	ResourcesTruncated          bool   `json:"resources_truncated,omitempty"`
	ImportCandidatesUnavailable string `json:"import_candidates_unavailable,omitempty"`
}

type queryResource struct {
	Address      string         `json:"address"`
	DisplayName  string         `json:"display_name"`
	Identity     map[string]any `json:"identity"`
	ResourceType string         `json:"resource_type"`
}

type queryListCompletion struct {
	Address      string `json:"address"`
	ResourceType string `json:"resource_type"`
	Total        int    `json:"total"`
}

type queryDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
}

type queryLogRecord struct {
	Type              string               `json:"type"`
	ListResourceFound *queryResource       `json:"list_resource_found"`
	ListComplete      *queryListCompletion `json:"list_complete"`
	Diagnostic        *queryDiagnostic     `json:"diagnostic"`
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
			mcp.WithString("resource_type", mcp.Description("Only return results of this resource type, for example aws_iam_role.")),
			mcp.WithString("address", mcp.Description("Only return results from this list block address, for example list.aws_iam_role.roles.")),
			mcp.WithString("name_contains", mcp.Description("Only return results whose display name contains this text (case-insensitive). Tag and attribute filtering are not supported.")),
			mcp.WithNumber("limit", mcp.Description("Results per page. Default 100, maximum 200.")),
			mcp.WithString("after", mcp.Description("Pass next_cursor from the previous page to read the next page of the same query run.")),
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
		return toolErrorf(logger, "get_query_summary", "failed to get Terraform client: %v", err)
	}
	id := strings.TrimSpace(queryRunID)
	filter := importworkflow.DiscoveryFilter{
		ResourceType: strings.TrimSpace(request.GetString("resource_type", "")),
		Address:      strings.TrimSpace(request.GetString("address", "")),
		NameContains: strings.TrimSpace(request.GetString("name_contains", "")),
		Limit:        request.GetInt("limit", 0),
		After:        strings.TrimSpace(request.GetString("after", "")),
	}
	if _, bad := request.GetArguments()["include_import_candidates"]; bad {
		return toolErrorf(logger, "get_query_summary", "include_import_candidates was removed: results are returned as selectable pages by default; use limit, after and the filters")
	}

	pageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page, err := importworkflow.ReadDiscoveryPage(pageCtx, tfeClient, id, filter)
	if err == nil {
		encoded, merr := json.Marshal(page)
		if merr != nil {
			return toolErrorf(logger, "get_query_summary", "failed to encode query summary: %v", merr)
		}
		return mcp.NewToolResultStructured(page, string(encoded)), nil
	}
	code := importworkflow.DiagnosticCode(err)
	if filter.After != "" {
		return mcp.NewToolResultError(code), nil
	}

	// Not a complete finished no-code query (for example an errored one):
	// return the bounded diagnostic summary with the reason.
	summary, err := readQuerySummaryData(ctx, tfeClient, id)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "failed to get query summary for %q: %v", queryRunID, err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > importworkflow.MaxDiscoveryPageSize {
		limit = importworkflow.DefaultDiscoveryPageSize
	}
	if len(summary.Resources) > limit {
		summary.Resources = summary.Resources[:limit]
		summary.ResourcesTruncated = true
	}
	summary.ImportCandidatesUnavailable = code
	response, err := json.Marshal(summary)
	if err != nil {
		return toolErrorf(logger, "get_query_summary", "marshaling query summary: %v", err)
	}
	return mcp.NewToolResultText(string(response)), nil
}

func readQuerySummaryData(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (*querySummary, error) {
	logReader, err := tfeClient.QueryRuns.Logs(ctx, queryRunID)
	if err != nil {
		return nil, err
	}
	return parseQuerySummary(logReader)
}

func readQuerySummary(ctx context.Context, tfeClient *tfe.Client, queryRunID string) (string, error) {
	summary, err := readQuerySummaryData(ctx, tfeClient, queryRunID)
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
	summary := &querySummary{
		Resources:       []queryResource{},
		ListCompletions: []queryListCompletion{},
		Diagnostics:     []queryDiagnostic{},
	}
	lines := bufio.NewReader(reader)

	for {
		line, err := lines.ReadBytes('\n')
		if len(line) > 0 {
			var record queryLogRecord
			if json.Unmarshal(line, &record) == nil {
				switch {
				case record.Type == "list_resource_found" && record.ListResourceFound != nil:
					summary.Resources = append(summary.Resources, *record.ListResourceFound)
				case record.Type == "list_complete" && record.ListComplete != nil:
					summary.ResourcesDiscovered += record.ListComplete.Total
					summary.ListCompletions = append(summary.ListCompletions, *record.ListComplete)
				case record.Type == "diagnostic" && record.Diagnostic != nil:
					summary.Diagnostics = append(summary.Diagnostics, *record.Diagnostic)
				}
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

const getQuerySummaryDescription = `Retrieves and parses the NDJSON log for an HCP Terraform query run.

Call get_query_status first and wait for it to return a terminal status, then pass the
same query_run_id to this tool.

For a finished, complete no-code query the result is one bounded page of selectable
results grouped by list block (lists[]): each group states address and resource_type once and
shared_identity (identity keys equal in every row of the group); each row has candidate_id,
display_name, tags when the result has any (resource attributes come from the query's generated configuration, so the query must have run with generate_config_out true; rows_without_attributes and the notes say when attributes were not captured, in which case an absent tags field does not mean the resource has no tags) and only the identity keys that differ, so merge shared_identity with the row's
identity. Use the tags to choose resources; filter on them yourself, there is no tag filter. The result also has resources_discovered, by_type counts, total_matching, a log_digest
and, when more rows match, next_cursor. Default 100 rows per page, maximum 200. Narrow with resource_type, address or name_contains; read the next
page by passing next_cursor as after. A page is a view of a fully checked query. If the
log changes between pages the call returns snapshot_changed_restart_paging. A query may
contain more than 100 results; select up to 100 candidate_id values from any page, then
call prepare_import only for the candidates to import. A query only sees what its filters and list arguments cover, so more matching resources may exist beyond the results; a list that returns exactly 100 results has hit Terraform's default limit and may have been cut off. Tag and attribute filtering are not supported.

If the query is not finished, errored or incomplete, the result is a bounded diagnostic
summary (resources_discovered, resources, list_completions and Terraform diagnostics)
with import_candidates_unavailable naming the reason; it has no selectable candidates.`
