// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuerySummaryDefinition(t *testing.T) {
	tool := GetQuerySummary(silentLogger())

	assert.Equal(t, "get_query_summary", tool.Tool.Name)
	assert.Contains(t, tool.Tool.Description, "Search-only callers need no import target")
	assert.Contains(t, tool.Tool.Description, "If the user asks to import")
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_run_id")
	assert.Contains(t, tool.Tool.Description, "get_query_status")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
}

func TestParseQuerySummary(t *testing.T) {
	queryLog := strings.Join([]string{
		`{"type":"list_start","list_start":{"address":"list.aws_instance.example"}}`,
		`not JSON`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_instance.example","display_name":"i-1234567890abcdef0","identity":{"id":"i-1234567890abcdef0"},"identity_version":1,"resource_type":"aws_instance"}}`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_bucket.example","display_name":"example-bucket","identity":{"bucket":"example-bucket","region":"us-east-2"},"identity_version":1,"resource_type":"aws_s3_bucket"}}`,
		`{"type":"diagnostic","diagnostic":{"severity":"warning","summary":"Partial result","detail":"One resource could not be read."}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_instance.example","resource_type":"aws_instance","total":2}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_bucket.example","resource_type":"aws_s3_bucket","total":3}}`,
	}, "\r\n")

	summary, err := parseQuerySummary(strings.NewReader(queryLog))

	require.NoError(t, err)
	assert.Equal(t, 5, summary.ResourcesDiscovered)
	assert.Equal(t, []queryResource{
		{
			Address:      "list.aws_instance.example",
			DisplayName:  "i-1234567890abcdef0",
			Identity:     map[string]any{"id": "i-1234567890abcdef0"},
			ResourceType: "aws_instance",
		},
		{
			Address:      "list.aws_s3_bucket.example",
			DisplayName:  "example-bucket",
			Identity:     map[string]any{"bucket": "example-bucket", "region": "us-east-2"},
			ResourceType: "aws_s3_bucket",
		},
	}, summary.Resources)
	assert.Equal(t, []queryListCompletion{
		{Address: "list.aws_instance.example", ResourceType: "aws_instance", Total: 2},
		{Address: "list.aws_s3_bucket.example", ResourceType: "aws_s3_bucket", Total: 3},
	}, summary.ListCompletions)
	assert.Equal(t, []queryDiagnostic{
		{Severity: "warning", Summary: "Partial result", Detail: "One resource could not be read."},
	}, summary.Diagnostics)
}

func TestParseQuerySummaryWithoutListCompletions(t *testing.T) {
	summary, err := parseQuerySummary(strings.NewReader(`{"type":"list_start"}`))

	require.NoError(t, err)
	assert.Zero(t, summary.ResourcesDiscovered)
	assert.Empty(t, summary.Resources)
	assert.Empty(t, summary.ListCompletions)
	assert.Empty(t, summary.Diagnostics)
}

func TestReadQuerySummary(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","terraform-version":"1.14.0","generate-config-out":false,"log-read-url":%q}}}`, server.URL+"/logs")
		case "/logs":
			logBytes := []byte("\x02{\"type\":\"list_resource_found\",\"list_resource_found\":{\"address\":\"list.aws_instance.example\",\"display_name\":\"i-1234567890abcdef0\",\"identity\":{\"id\":\"i-1234567890abcdef0\"},\"resource_type\":\"aws_instance\"}}\n{\"type\":\"diagnostic\",\"diagnostic\":{\"severity\":\"error\",\"summary\":\"Query failed\",\"detail\":\"Provider returned an error.\"}}\n{\"type\":\"list_complete\",\"list_complete\":{\"address\":\"list.aws_instance.example\",\"resource_type\":\"aws_instance\",\"total\":2}}\n\x03")
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(logBytes) {
				end := min(offset+limit, len(logBytes))
				_, _ = w.Write(logBytes[offset:end])
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "test-token", HTTPClient: server.Client()})
	require.NoError(t, err)

	response, err := readQuerySummary(context.Background(), tfeClient, "qry-test")

	require.NoError(t, err)
	assert.JSONEq(t, `{"resources_discovered":2,"resources":[{"address":"list.aws_instance.example","display_name":"i-1234567890abcdef0","identity":{"id":"i-1234567890abcdef0"},"resource_type":"aws_instance"}],"list_completions":[{"address":"list.aws_instance.example","resource_type":"aws_instance","total":2}],"diagnostics":[{"severity":"error","summary":"Query failed","detail":"Provider returned an error."}]}`, response)
}

// The diagnostic fallback is never selectable, including for a QueryRun whose
// found-resource records lack identity_version (the whole run is invalid, not
// partly selectable). Its resources carry no candidate_id.
func TestGetQuerySummaryFallbackIsExplicitlyNotSelectable(t *testing.T) {
	fixture, err := os.ReadFile("testdata/import/same-version/backend.json")
	require.NoError(t, err)
	var responses map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fixture, &responses))
	responses["/api/v2/workspaces/ws-fixture"] = responses["/api/v2/organizations/fixture-org/workspaces/import-root"]
	logBytes := []byte("\x02" + strings.Join([]string{
		`{"type":"list_start","list_start":{"address":"list.aws_iam_role.roles","resource_type":"aws_iam_role"}}`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_iam_role.roles","display_name":"role-0","identity":{"name":"role-0"},"identity_version":0,"resource_type":"aws_iam_role"}}`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_iam_role.roles","display_name":"role-1","identity":{"name":"role-1"},"resource_type":"aws_iam_role"}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_iam_role.roles","resource_type":"aws_iam_role","total":2}}`,
	}, "\n") + "\n\x03")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/logs":
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(logBytes) {
				_, _ = w.Write(logBytes[offset:min(offset+limit, len(logBytes))])
			}
		default:
			body, ok := responses[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.api+json")
			if r.URL.Path == "/api/v2/queries/qry-fixture" {
				var query map[string]any
				require.NoError(t, json.Unmarshal(body, &query))
				query["data"].(map[string]any)["attributes"].(map[string]any)["log-read-url"] = server.URL + "/logs"
				body, _ = json.Marshal(query)
			}
			_, _ = w.Write(body)
		}
	}))
	defer server.Close()
	t.Setenv(client.TerraformAddress, server.URL)
	t.Setenv(client.TerraformToken, "test-token")

	result, err := getQuerySummaryHandler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"query_run_id": "qry-fixture"}}}, silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)

	var wire struct {
		Selectable                  *bool            `json:"selectable"`
		NextAction                  string           `json:"next_action"`
		ImportCandidatesUnavailable string           `json:"import_candidates_unavailable"`
		Resources                   []map[string]any `json:"resources"`
	}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &wire))
	require.NotNil(t, wire.Selectable, "the fallback states selectability explicitly")
	assert.False(t, *wire.Selectable)
	assert.Equal(t, "query_identity_version_invalid", wire.ImportCandidatesUnavailable)
	assert.Contains(t, wire.NextAction, "not a selectable result")
	assert.Contains(t, wire.NextAction, "whole query run is not selectable import evidence")
	assert.Contains(t, wire.NextAction, "treat any part of this query run as selectable")
	require.Len(t, wire.Resources, 2)
	for _, resource := range wire.Resources {
		assert.NotContains(t, resource, "candidate_id")
	}
}

func TestDiagnosticSummaryNextActionForInvalidIdentityVersion(t *testing.T) {
	next := diagnosticSummaryNextAction("query_identity_version_invalid")
	assert.Contains(t, next, "not a selectable result")
	assert.Contains(t, next, "the whole query run is not selectable import evidence")
	assert.Contains(t, next, "0 is valid")
	assert.NotContains(t, next, "import_candidates_unavailable names the reason")
	other := diagnosticSummaryNextAction("query_evidence_unverified")
	assert.Contains(t, other, "not a selectable result")
	assert.Contains(t, other, "import_candidates_unavailable names the reason")
	assert.Contains(t, GetQuerySummary(silentLogger()).Tool.Description, "selectable false")
	assert.Contains(t, GetQuerySummary(silentLogger()).Tool.Description, "query_identity_version_invalid")
}
