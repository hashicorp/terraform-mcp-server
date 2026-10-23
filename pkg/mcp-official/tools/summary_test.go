// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	legacymcp "github.com/mark3labs/mcp-go/mcp"
	legacyserver "github.com/mark3labs/mcp-go/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// Retains the source selectable/version-zero, missing-version, and failed-query
// acceptance cases, and extends them to cross-adapter structured/text parity.
func TestSearchSummaryTransportParity(t *testing.T) {
	valid := summaryLogFixture("0")
	for _, tc := range []struct {
		name, status, body, filter, diagnostic string
		selectable, toolError                  bool
	}{
		{name: "valid version zero", status: "finished", body: valid, selectable: true},
		{name: "case insensitive filter", status: "finished", body: valid, filter: "FIRST", selectable: true},
		{name: "no matching rows", status: "finished", body: valid, filter: "absent", selectable: true},
		{name: "missing identity version", status: "finished", body: summaryLogFixture("null"), diagnostic: "query_identity_version_invalid"},
		{name: "terminal failed query", status: "errored", body: valid, diagnostic: "query_not_selectable"},
		{name: "terminal canceled query", status: "canceled", body: valid, diagnostic: "query_not_selectable"},
		{name: "incomplete query", status: "finished", body: strings.Replace(valid, `"total":1`, `"total":2`, 1), diagnostic: "query_evidence_incomplete"},
		{name: "late error outside filter", status: "finished", body: valid + `{"type":"diagnostic","diagnostic":{"severity":"error","detail":"backend-secret-sentinel"}}`, filter: "absent", diagnostic: "query_reported_error"},
		{name: "late malformed progress", status: "finished", body: valid + `{"type":"provider_installation_start",`, diagnostic: "query_evidence_invalid", toolError: true},
		{name: "invalid generated HCL", status: "finished", body: strings.Replace(valid, `bucket = \"first\" }`, `bucket = \"first\" `, 1), diagnostic: "query_generated_configuration_invalid"},
		{name: "nonmatching HCL is not projected", status: "finished", body: strings.Replace(valid, `bucket = \"first\" }`, `bucket = \"first\" `, 1), filter: "absent", selectable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, logReads := summaryReadOnlyBackend(t, tc.status, "org", tc.body, "")
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
			ctx, session := newOfficialSearchSession(t, middleware.OrganizationAllowlist([]string{"org"}, slog.Default()))
			args := map[string]any{"query_run_id": "  qry-test\t", "resource_type": "aws_s3_buckets", "address": "list.aws_s3_buckets.all", "name_contains": tc.filter}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_summary", Arguments: args})
			require.NoError(t, err)
			require.Equal(t, tc.toolError, result.IsError)
			var logs bytes.Buffer
			logger := log.New()
			logger.SetOutput(&logs)
			// Scope this assertion to tool errors, not the HTTP client's request logs.
			logger.SetLevel(log.ErrorLevel)
			legacy := searchlogic.GetQuerySummary(logger)
			legacyCtx := legacyserver.NewMCPServer("summary-test", "0").WithContext(ctx, summaryTestSession{})
			legacyCtx = terraformclient.WithOrganizationAllowlist(legacyCtx, []string{"org"})
			old, err := legacy.Handler(legacyCtx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Name: "get_query_summary", Arguments: args}})
			require.NoError(t, err)
			require.Equal(t, result.IsError, old.IsError)
			text := result.Content[0].(*mcp.TextContent).Text
			require.Equal(t, old.Content[0].(legacymcp.TextContent).Text, text)
			for _, forbidden := range []string{"resource_object", "resource-object-secret-sentinel", "must-not-be-returned", "signed-secret-sentinel", "backend-secret-sentinel", backend.URL, `"configuration":`, "observed_attributes"} {
				require.NotContains(t, text, forbidden)
				require.NotContains(t, logs.String(), forbidden)
			}
			if tc.toolError {
				require.Nil(t, result.StructuredContent)
				require.Nil(t, old.StructuredContent)
				require.Contains(t, text, tc.diagnostic)
				return
			}
			encoded, err := json.Marshal(result.StructuredContent)
			require.NoError(t, err)
			require.JSONEq(t, text, string(encoded))
			oldEncoded, err := json.Marshal(old.StructuredContent)
			require.NoError(t, err)
			require.JSONEq(t, text, string(oldEncoded))
			structured := result.StructuredContent.(map[string]any)
			require.Equal(t, tc.selectable, structured["selectable"])
			require.Equal(t, true, structured["generate_config_out"])
			require.Equal(t, "org", structured["source_organization_name"])
			if tc.selectable && tc.filter != "absent" {
				require.Contains(t, text, `"query_result_marker":"qrm1-`)
				rows := structured["lists"].([]any)[0].(map[string]any)["results"].([]any)
				require.Equal(t, true, rows[0].(map[string]any)["configuration_generated"])
				require.Equal(t, "first", rows[0].(map[string]any)["configuration_attributes"].(map[string]any)["bucket"])
				require.Contains(t, text, `"configuration_attributes":{"bucket":"first"}`)
			} else {
				require.NotContains(t, text, `"query_result_marker":`)
				require.NotContains(t, text, `"first"`)
				if !tc.selectable {
					require.Equal(t, "nonselectable", structured["status"])
					require.Contains(t, text, tc.diagnostic)
				}
			}
			if tc.status != "finished" {
				require.Zero(t, logReads.Load(), "failed/canceled queries must not read evidence")
			}
		})
	}
}

func TestSearchSummarySafeBackendErrors(t *testing.T) {
	for _, tc := range []struct{ name, organization, failPath, diagnostic string }{
		{"query unavailable", "org", "/api/v2/queries/qry-test", "query_run_unavailable"},
		{"provenance unavailable", "org", "/api/v2/search/no-code-query/ncqry-test", "query_backend_unavailable"},
		{"logs unavailable", "org", "/logs", "query_log_unavailable"},
		{"denied organization", "blocked-secret-sentinel", "", "organization"},
		{"ownership unavailable", "", "", "query_ownership_unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, reads := summaryReadOnlyBackend(t, "finished", tc.organization, summaryLogFixture("0"), tc.failPath)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
			ctx, session := newOfficialSearchSession(t, middleware.OrganizationAllowlist([]string{"org"}, slog.Default()))
			args := map[string]any{"query_run_id": "qry-test"}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_query_summary", Arguments: args})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			var logs bytes.Buffer
			logger := log.New()
			logger.SetOutput(&logs)
			logger.SetLevel(log.ErrorLevel)
			legacyCtx := legacyserver.NewMCPServer("summary-test", "0").WithContext(ctx, summaryTestSession{})
			legacyCtx = terraformclient.WithOrganizationAllowlist(legacyCtx, []string{"org"})
			old, err := searchlogic.GetQuerySummary(logger).Handler(legacyCtx, legacymcp.CallToolRequest{Params: legacymcp.CallToolParams{Arguments: args}})
			require.NoError(t, err)
			require.True(t, old.IsError)
			require.Nil(t, old.StructuredContent)
			text := result.Content[0].(*mcp.TextContent).Text
			require.Equal(t, old.Content[0].(legacymcp.TextContent).Text, text)
			require.Contains(t, text, tc.diagnostic)
			for _, rendered := range []string{text, logs.String()} {
				require.NotContains(t, rendered, "secret-sentinel")
				require.NotContains(t, rendered, backend.URL)
				require.NotContains(t, rendered, "query_result_marker")
			}
			require.Zero(t, reads.Load())
		})
	}
}
