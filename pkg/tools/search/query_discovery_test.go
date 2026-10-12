// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/go-tfe"
	mcpclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

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

func TestPageQueryDiscoveryPreservesMarkersAndSnapshot(t *testing.T) {
	prov, evidence := testDiscovery(t, "first", "second", "third")
	page, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 2})
	require.NoError(t, err)
	require.True(t, page.Selectable)
	require.Equal(t, "org", page.SourceOrganizationName)
	require.Equal(t, 3, page.TotalMatching)
	require.Equal(t, 1, page.Remaining)
	require.True(t, page.HasMore)
	require.Equal(t, evidence.Results[0].Marker, page.Lists[0].Results[0].Marker)
	continuation, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 2, After: page.NextCursor})
	require.NoError(t, err)
	require.Equal(t, 1, continuation.Returned)
	require.Equal(t, evidence.Results[2].Marker, continuation.Lists[0].Results[0].Marker)
	require.False(t, continuation.HasMore)
	_, err = pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 2, After: page.NextCursor, NameContains: "first"})
	require.EqualError(t, err, "query_cursor_invalid")
	evidence.LogDigest = "sha256:changed"
	_, err = pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 2, After: page.NextCursor})
	require.EqualError(t, err, "query_snapshot_changed")
}

func TestPageQueryDiscoveryProjectsTagsAndExplainsMissingAttributes(t *testing.T) {
	prov, evidence := testDiscovery(t, "captured", "missing", "untagged")
	evidence.Results[0].ResourceObject = map[string]any{"tags": map[string]any{"team": "search", "enabled": true, "count": json.Number("2"), "nested": map[string]any{"private": "value"}}}
	evidence.Results[2].ResourceObject = map[string]any{"name": "untagged"}
	page, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 1, page.RowsWithoutAttributes)
	require.True(t, page.HasMore)
	require.Equal(t, map[string]any{"team": "search", "enabled": true, "count": json.Number("2")}, page.Lists[0].Results[0].Tags)
	require.True(t, page.Lists[0].Results[0].AttributesCaptured)
	require.Contains(t, page.NextAction, "same filters until has_more is false")
	require.Contains(t, page.NextAction, "any tags, including in script output")
	continuation, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{Limit: 2, After: page.NextCursor})
	require.NoError(t, err)
	require.Equal(t, 1, continuation.RowsWithoutAttributes)
	require.False(t, continuation.Lists[0].Results[0].AttributesCaptured)
	require.Empty(t, continuation.Lists[0].Results[0].Tags)
	require.True(t, continuation.Lists[0].Results[1].AttributesCaptured)
	require.Empty(t, continuation.Lists[0].Results[1].Tags)
	require.Contains(t, continuation.NextAction, "For rows with attributes_captured false, tags are unknown")
	require.Contains(t, continuation.NextAction, "ask before a new query")
	text, err := FormatQueryDiscoveryPage(continuation)
	require.NoError(t, err)
	require.Contains(t, text, `"attributes_captured":false`)
	require.NotContains(t, text, "private")
}

func TestPageQueryDiscoveryStopsOnOversizeFirstRow(t *testing.T) {
	prov, evidence := testDiscovery(t, "first")
	evidence.Results[0].Identity["large"] = strings.Repeat("x", maxQueryPageBytes)
	page, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.Nil(t, page)
	require.EqualError(t, err, "query_page_oversized")
}

func TestPageQueryDiscoveryOversizeLaterRowBlocksFirstPage(t *testing.T) {
	prov, evidence := testDiscovery(t, "first", "second", "third")
	evidence.Results[1].Identity["large"] = strings.Repeat("x", maxQueryPageBytes)
	page, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.Nil(t, page)
	require.EqualError(t, err, "query_page_oversized")
}

func TestPageQueryDiscoveryNeverProjectsSignedArtifact(t *testing.T) {
	prov, evidence := testDiscovery(t, "first")
	evidence.Results[0].Identity["url"] = "https://storage.example/path?X-Amz-Signature=sentinel"
	page, err := pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.Nil(t, page)
	require.EqualError(t, err, "query_sensitive_result")
	evidence.Results[0].Identity["url"] = "https://example.com/resources/one"
	page, err = pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/resources/one", page.Lists[0].Results[0].Identity["url"])
	evidence.Results[0].ResourceObject = map[string]any{"tags": map[string]any{"link": "https://storage.example/log?X-Amz-Signature=sentinel"}}
	page, err = pageQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.Nil(t, page)
	require.EqualError(t, err, "query_sensitive_result")
}

func TestQueryProvenanceChecksBackendAssociations(t *testing.T) {
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
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
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "other-org")
	prov, err = readQueryDiscoveryProvenance(context.Background(), client, "qry-test")
	require.Nil(t, prov)
	require.EqualError(t, err, "query_organization_not_allowed")
}

func TestLegacyQuerySummaryProjectsBoundedSelectableResults(t *testing.T) {
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
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
	page, ok := result.StructuredContent.(*QueryDiscoveryPage)
	require.True(t, ok)
	require.True(t, page.Selectable)
	require.Equal(t, "org", page.SourceOrganizationName)
	require.Len(t, page.Lists, 1)
	require.Regexp(t, `^qrm1-`, page.Lists[0].Results[0].Marker)
	require.Equal(t, map[string]any{"team": "search"}, page.Lists[0].Results[0].Tags)
	require.True(t, page.Lists[0].Results[0].AttributesCaptured)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"query_result_marker"`)
	require.Contains(t, text.Text, `"tags":{"team":"search"}`)
	require.NotContains(t, text.Text, "/logs")
}

func TestQuerySummaryReadsLargeLogInFewRequestsWithoutReturningSignedURL(t *testing.T) {
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, fmt.Sprintf(`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"bucket-%d","identity":{"bucket":"bucket-%d"},"identity_version":0}}`, i, i))
	}
	lines = append(lines, `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":3000}}`)
	logData := []byte("\x02" + strings.Join(lines, "\n") + "\n\x03")
	require.Greater(t, len(logData), 400<<10)
	const signedQuery = "X-Amz-Signature=secret-sentinel"
	var requests int
	var denyLog atomic.Bool
	var capLog atomic.Bool
	var leakedHeader atomic.Bool
	var missingSignature atomic.Bool
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs?"+signedQuery)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		case "/logs":
			requests++
			for _, name := range []string{"Authorization", "X-Forwarded-For", "X-Test-Secret", mcpclient.SharedSecretHeader} {
				if r.Header.Get(name) != "" {
					leakedHeader.Store(true)
				}
			}
			if r.URL.Query().Get("X-Amz-Signature") != "secret-sentinel" {
				missingSignature.Store(true)
			}
			if denyLog.Load() {
				http.Error(w, "log unavailable: "+backend.URL+"/logs?"+signedQuery, http.StatusForbidden)
				return
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(logData) {
				end := min(len(logData), offset+limit)
				if capLog.Load() {
					end = min(end, offset+(64<<10))
				}
				// Force many small network body reads per request. The fast path
				// must drain the response, not treat each read as a new range.
				for start := offset; start < end; start += 4096 {
					_, _ = w.Write(logData[start:min(end, start+4096)])
					w.(http.Flusher).Flush()
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client(), Headers: http.Header{"X-Test-Secret": {"private-header-sentinel"}, "X-Forwarded-For": {"127.0.0.1"}, mcpclient.SharedSecretHeader: {"shared-secret-sentinel"}}})
	require.NoError(t, err)
	result, err := getQuerySummaryWithClient(context.Background(), client, "qry-test", QueryDiscoveryFilter{Limit: 2}, silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError)
	page := result.StructuredContent.(*QueryDiscoveryPage)
	require.Equal(t, 3000, page.ResourcesDiscovered)
	require.True(t, page.HasMore)
	require.NotEmpty(t, page.NextCursor)
	t.Logf("log bytes: %d; log GETs: %d", len(logData), requests)
	require.Equal(t, 1, requests, "a large streamed response must be fully consumed in one GET")
	require.False(t, leakedHeader.Load(), "API bearer and custom headers must not go to the signed log host")
	require.False(t, missingSignature.Load(), "preserve the backend's signed query parameters")
	require.NotContains(t, fmt.Sprint(result), signedQuery)
	require.NotContains(t, fmt.Sprint(result), "/logs")
	capLog.Store(true)
	second, err := getQuerySummaryWithClient(context.Background(), client, "qry-test", QueryDiscoveryFilter{Limit: 2, After: page.NextCursor}, silentLogger())
	require.NoError(t, err)
	require.False(t, second.IsError)
	require.Equal(t, 3000, second.StructuredContent.(*QueryDiscoveryPage).ResourcesDiscovered)
	require.Less(t, requests, 16, "capped responses should need a few large ranged GETs")
	denyLog.Store(true)
	blocked, err := getQuerySummaryWithClient(context.Background(), client, "qry-test", QueryDiscoveryFilter{}, silentLogger())
	require.NoError(t, err)
	require.True(t, blocked.IsError)
	require.NotContains(t, fmt.Sprint(blocked), signedQuery)
	require.NotContains(t, fmt.Sprint(blocked), "/logs")
}
