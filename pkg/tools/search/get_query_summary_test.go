// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuerySummaryDefinition(t *testing.T) {
	tool := GetQuerySummary(silentLogger())

	assert.Equal(t, "get_query_summary", tool.Tool.Name)
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
		`{"type":"list_complete","list_complete":{"address":"list.aws_instance.example","resource_type":"aws_instance","total":2}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_bucket.example","resource_type":"aws_s3_bucket","total":3}}`,
	}, "\r\n")

	summary, err := parseQuerySummary(strings.NewReader(queryLog))

	require.NoError(t, err)
	assert.Equal(t, 5, summary.ResourcesDiscovered)
	assert.Equal(t, []queryListCompletion{
		{Address: "list.aws_instance.example", ResourceType: "aws_instance", Total: 2},
		{Address: "list.aws_s3_bucket.example", ResourceType: "aws_s3_bucket", Total: 3},
	}, summary.ListCompletions)
}

func TestParseQuerySummaryWithoutListCompletions(t *testing.T) {
	summary, err := parseQuerySummary(strings.NewReader(`{"type":"list_start"}`))

	require.NoError(t, err)
	assert.Zero(t, summary.ResourcesDiscovered)
	assert.Empty(t, summary.ListCompletions)
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
			logBytes := []byte("\x02{\"type\":\"list_complete\",\"list_complete\":{\"address\":\"list.aws_instance.example\",\"resource_type\":\"aws_instance\",\"total\":2}}\n\x03")
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
	assert.JSONEq(t, `{"resources_discovered":2,"list_completions":[{"address":"list.aws_instance.example","resource_type":"aws_instance","total":2}]}`, response)
}
