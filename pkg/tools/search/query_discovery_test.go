// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNonselectableQueryEvidenceCode(t *testing.T) {
	for _, expected := range []queryEvidenceCode{
		queryNotSelectableCode,
		queryIdentityVersionInvalidCode,
		queryEvidenceIncompleteCode,
		queryDuplicateOrInvalidIdentityCode,
		queryIdentityInvalidCode,
		queryReportedErrorCode,
		queryGeneratedBlockConflictCode,
	} {
		t.Run(string(expected), func(t *testing.T) {
			actual, ok := nonselectableQueryEvidenceCode(fmt.Errorf("wrapped: %w", queryEvidenceError(expected)))
			require.True(t, ok)
			require.Equal(t, expected, actual)
		})
	}

	for _, err := range []error{errors.New("query_not_selectable"), queryEvidenceError(queryProviderSourceUnresolvedCode)} {
		code, ok := nonselectableQueryEvidenceCode(err)
		require.False(t, ok)
		require.Empty(t, code)
	}
}

func testDiscovery(t *testing.T, ids ...string) (*queryDiscoveryProvenance, *queryEvidence) {
	t.Helper()
	var lines []string
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf(`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":%q,"identity":{"bucket":%q},"identity_version":0}}`, id, id))
	}
	lines = append(lines, fmt.Sprintf(`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":%d}}`, len(ids)))
	evidence, err := parseQueryEvidence("qry-test", []byte(strings.Join(lines, "\n")), testQueryProviders())
	require.NoError(t, err)
	return &queryDiscoveryProvenance{QueryRunID: "qry-test", Organization: "org", WorkspaceName: "ws", WorkspaceID: "ws-test"}, evidence
}

func TestSummarizeQueryDiscoveryReturnsEveryMatchingResult(t *testing.T) {
	prov, evidence := testDiscovery(t, "first", "second", "third")
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.True(t, summary.Selectable)
	require.Equal(t, "org", summary.SourceOrganizationName)
	require.Equal(t, 3, summary.TotalMatching)
	require.Len(t, summary.Lists[0].Results, 3)
	require.Equal(t, evidence.Results[0].Marker, summary.Lists[0].Results[0].Marker)
	require.Contains(t, summary.NextAction, "all results of this query")

	filtered, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{NameContains: "second"})
	require.NoError(t, err)
	require.Equal(t, 1, filtered.TotalMatching)
	require.Equal(t, evidence.Results[1].Marker, filtered.Lists[0].Results[0].Marker)
	require.Contains(t, filtered.NextAction, "all results matching the supplied summary filters")
}

func TestSummarizeQueryDiscoveryProjectsTagsAndExplainsMissingAttributes(t *testing.T) {
	prov, evidence := testDiscovery(t, "captured", "missing", "untagged")
	evidence.Results[0].ResourceObject = map[string]any{"tags": map[string]any{"team": "search", "enabled": true, "count": json.Number("2"), "nested": map[string]any{"private": "value"}}}
	evidence.Results[2].ResourceObject = map[string]any{"name": "untagged"}
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, summary.RowsWithoutAttributes)
	require.Equal(t, map[string]any{"team": "search", "enabled": true, "count": json.Number("2")}, summary.Lists[0].Results[0].Tags)
	require.True(t, summary.Lists[0].Results[0].AttributesCaptured)
	require.False(t, summary.Lists[0].Results[1].AttributesCaptured)
	require.Empty(t, summary.Lists[0].Results[1].Tags)
	require.True(t, summary.Lists[0].Results[2].AttributesCaptured)
	require.Empty(t, summary.Lists[0].Results[2].Tags)
	require.Contains(t, summary.NextAction, "For rows with attributes_captured false, tags are unknown")
	require.Contains(t, summary.NextAction, "ask before a new query")
	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.Contains(t, text, `"attributes_captured":false`)
	require.NotContains(t, text, "private")
}

func TestSummarizeQueryDiscoveryDoesNotApplyOutputSizeCaps(t *testing.T) {
	prov, evidence := testDiscovery(t, "large")
	evidence.Results[0].Identity["large"] = strings.Repeat("x", 100*1024)
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Len(t, summary.Lists[0].Results, 1)
	require.Len(t, summary.Lists[0].Results[0].Identity["large"], 100*1024)

	_, err = FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
}

func TestQueryProvenanceChecksBackendAssociations(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	prov, err := readQueryDiscoveryProvenance(context.Background(), client, "qry-test")
	require.NoError(t, err)
	require.Equal(t, "org", prov.Organization)
	require.Equal(t, "registry.terraform.io/hashicorp/aws", prov.Providers["aws_s3_buckets"].Source)
}

func TestLegacyQuerySummaryProjectsBoundedSelectableResults(t *testing.T) {
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs")
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		case "/logs":
			found := strings.Replace(string(queryEvidenceFixture("0")), `"identity_version":0`, `"identity_version":0,"resource_object":{"tags":{"team":"search"}}`, 1)
			data := append([]byte("\x02Terraform query started\n"+`{"type":"provider_installation_start","provider":"aws"}`+"\n"), []byte(found)...)
			data = append(data, 3)
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(data) {
				_, _ = w.Write(data[offset:min(len(data), offset+limit)])
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	result, err := getQuerySummaryWithClient(context.Background(), client, "qry-test", QueryDiscoveryFilter{}, silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError, "tool content: %#v", result.Content)
	summary, ok := result.StructuredContent.(*QueryDiscoverySummary)
	require.True(t, ok)
	require.True(t, summary.Selectable)
	require.Equal(t, "org", summary.SourceOrganizationName)
	require.Len(t, summary.Lists, 1)
	require.Regexp(t, `^qrm1-`, summary.Lists[0].Results[0].Marker)
	require.Equal(t, map[string]any{"team": "search"}, summary.Lists[0].Results[0].Tags)
	require.True(t, summary.Lists[0].Results[0].AttributesCaptured)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"query_result_marker"`)
	require.Contains(t, text.Text, `"tags":{"team":"search"}`)
	require.NotContains(t, text.Text, "/logs")
}
