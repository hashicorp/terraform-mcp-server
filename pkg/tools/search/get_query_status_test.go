// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQueryStatusDefinition(t *testing.T) {
	tool := GetQueryStatus(silentLogger())

	assert.Equal(t, "get_query_status", tool.Tool.Name)
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_run_id")
	assert.Contains(t, tool.Tool.Description, "go-tfe")
	assert.Contains(t, tool.Tool.Description, "pending")
	assert.Contains(t, tool.Tool.Description, "finished, errored, or canceled")
	assert.Contains(t, tool.Tool.Description, "same query_run_id")
	assert.Contains(t, tool.Tool.Description, "get_query_summary")
	assert.NotNil(t, tool.Tool.OutputSchema)
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Tool.Annotations.OpenWorldHint)
}

func TestWaitForQueryStatusPollsUntilFinished(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}

		status := "running"
		if reads.Add(1) > 1 {
			status = "finished"
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":%q,"terraform-version":"1.14.0","generate-config-out":false}}}`, status)
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "test-token", HTTPClient: server.Client()})
	require.NoError(t, err)

	response, err := waitForQueryStatus(context.Background(), tfeClient, "qry-test", time.Millisecond, queryStatusMaxReads)

	require.NoError(t, err)
	assert.Equal(t, tfe.QueryRunFinished, response.Status)
	assert.True(t, response.Terminal)
	assert.Contains(t, response.Message, "get_query_summary")
	assert.Equal(t, int32(2), reads.Load())
}

func TestWaitForQueryStatusReturnsNonterminalStatus(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		reads.Add(1)
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"type":"queries","id":"qry-test","attributes":{"status":"running","terraform-version":"1.14.0","generate-config-out":false}}}`))
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "test-token", HTTPClient: server.Client()})
	require.NoError(t, err)
	response, err := waitForQueryStatus(context.Background(), tfeClient, "qry-test", 2*time.Second, 2)

	require.NoError(t, err)
	assert.False(t, response.Terminal)
	assert.Equal(t, tfe.QueryRunStatus("running"), response.Status)
	assert.Equal(t, 2, response.RetryAfterSeconds)
	assert.Contains(t, response.Message, "same query_run_id")
	assert.Equal(t, int32(2), reads.Load())
}

func TestReadQueryStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}

		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/queries/qry-test", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"type":"queries","id":"qry-test","attributes":{"status":"running","terraform-version":"1.14.0","generate-config-out":false}}}`))
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{
		Address:    server.URL,
		Token:      "test-token",
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	response, err := readQueryStatus(context.Background(), tfeClient, "qry-test")

	require.NoError(t, err)
	assert.Contains(t, response, `"query_run_id":"qry-test"`)
	assert.Contains(t, response, `"status":"running"`)
}

func TestReadQueryStatusReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"Query run not found"}]}`))
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "test-token", HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = readQueryStatus(context.Background(), tfeClient, "qry-missing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource not found")
}
