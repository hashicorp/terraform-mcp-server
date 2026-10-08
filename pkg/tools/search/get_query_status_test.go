// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type queryStatusTestSession struct {
	id string
}

type queryRunsStub struct {
	tfe.QueryRuns
	read func(context.Context, string) (*tfe.QueryRun, error)
}

func (s queryRunsStub) Read(ctx context.Context, queryRunID string) (*tfe.QueryRun, error) {
	return s.read(ctx, queryRunID)
}

type queryStatusWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *queryStatusWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func (queryStatusTestSession) Initialize() {}

func (queryStatusTestSession) Initialized() bool { return true }

func (queryStatusTestSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }

func (s queryStatusTestSession) SessionID() string { return s.id }

func TestGetQueryStatusDefinition(t *testing.T) {
	tool := GetQueryStatus(silentLogger())

	assert.Equal(t, "get_query_status", tool.Tool.Name)
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_run_id")
	assert.Contains(t, tool.Tool.Description, "go-tfe")
	assert.Contains(t, tool.Tool.Description, "pending")
	assert.Contains(t, tool.Tool.Description, "finished, errored, or canceled")
	assert.Contains(t, tool.Tool.Description, "same query_run_id")
	assert.Contains(t, tool.Tool.Description, "get_query_summary")
	assert.Contains(t, tool.Tool.Description, "every three seconds")
	assert.Contains(t, tool.Tool.Description, "at most 40 seconds")
	assert.Contains(t, tool.Tool.Description, "retry_after_seconds is omitted")
	assert.Equal(t, "object", tool.Tool.OutputSchema.Type)
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "query_run_id")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "status")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "terminal")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "retry_after_seconds")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "message")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "status_timestamps")
	assert.ElementsMatch(t, []string{"query_run_id", "status", "terminal", "message"}, tool.Tool.OutputSchema.Required)
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

	ctx := context.Background()
	response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", time.Millisecond, nil)

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
	ctx, cancel := context.WithTimeoutCause(context.Background(), 10*time.Millisecond, errQueryStatusWaitBudgetExceeded)
	defer cancel()
	response, err := waitForQueryStatus(context.Background(), ctx, tfeClient, "qry-test", time.Second, nil)

	require.NoError(t, err)
	assert.False(t, response.Terminal)
	assert.Equal(t, tfe.QueryRunStatus("running"), response.Status)
	assert.Equal(t, 5, response.RetryAfterSeconds)
	assert.Contains(t, response.Message, "same query_run_id")
	assert.Equal(t, int32(1), reads.Load())
}

func TestWaitForQueryStatusReturnsEachTerminalStatus(t *testing.T) {
	for _, status := range []tfe.QueryRunStatus{tfe.QueryRunFinished, tfe.QueryRunErrored, tfe.QueryRunCanceled} {
		t.Run(string(status), func(t *testing.T) {
			var reads atomic.Int32
			server := queryStatusTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				reads.Add(1)
				writeQueryStatusResponse(w, status)
			})
			tfeClient := queryStatusTFEClient(t, server)

			ctx := context.Background()
			response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", time.Millisecond, nil)

			require.NoError(t, err)
			assert.Equal(t, status, response.Status)
			assert.True(t, response.Terminal)
			assert.Zero(t, response.RetryAfterSeconds)
			assert.Contains(t, response.Message, "get_query_summary")
			assert.Equal(t, int32(1), reads.Load())
		})
	}
}

func TestWaitForQueryStatusReturnsLastStatusWhenInternalBudgetExpires(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	tfeClient := queryStatusStubClient(func(context.Context, string) (*tfe.QueryRun, error) {
		cancel(errQueryStatusWaitBudgetExceeded)
		return &tfe.QueryRun{ID: "qry-test", Status: tfe.QueryRunRunning}, nil
	})

	response, err := waitForQueryStatus(context.Background(), ctx, tfeClient, "qry-test", time.Second, nil)

	require.NoError(t, err)
	assert.Equal(t, tfe.QueryRunRunning, response.Status)
	assert.False(t, response.Terminal)
	assert.Equal(t, queryStatusRetryAfterSeconds, response.RetryAfterSeconds)
}

func TestWaitForQueryStatusPreservesCallerCancellationWhileWaiting(t *testing.T) {
	callerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)
	pollCtx := &queryStatusWaitContext{Context: callerCtx, waiting: waiting}
	reads := 0
	tfeClient := queryStatusStubClient(func(context.Context, string) (*tfe.QueryRun, error) {
		reads++
		return &tfe.QueryRun{ID: "qry-test", Status: tfe.QueryRunRunning}, nil
	})
	go func() {
		select {
		case <-waiting:
			cancel()
		case <-stop:
		}
	}()

	response, err := waitForQueryStatus(callerCtx, pollCtx, tfeClient, "qry-test", time.Second, nil)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, reads)
}

func TestWaitForQueryStatusReturnsLastStatusWhenBudgetExpiresDuringRead(t *testing.T) {
	var reads atomic.Int32
	server := queryStatusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) == 1 {
			writeQueryStatusResponse(w, tfe.QueryRunRunning)
			return
		}
		<-r.Context().Done()
	})
	tfeClient := queryStatusTFEClient(t, server)
	ctx, cancel := context.WithTimeoutCause(context.Background(), 100*time.Millisecond, errQueryStatusWaitBudgetExceeded)
	defer cancel()

	response, err := waitForQueryStatus(context.Background(), ctx, tfeClient, "qry-test", time.Millisecond, nil)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, tfe.QueryRunRunning, response.Status)
	assert.False(t, response.Terminal)
	assert.Equal(t, queryStatusRetryAfterSeconds, response.RetryAfterSeconds)
	assert.Equal(t, int32(2), reads.Load())
}

func TestWaitForQueryStatusPreservesInitialReadFailureWhenBudgetExpires(t *testing.T) {
	server := queryStatusTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	tfeClient := queryStatusTFEClient(t, server)
	ctx, cancel := context.WithTimeoutCause(context.Background(), 100*time.Millisecond, errQueryStatusWaitBudgetExceeded)
	defer cancel()

	response, err := waitForQueryStatus(context.Background(), ctx, tfeClient, "qry-test", time.Second, nil)

	assert.Nil(t, response)
	assert.Error(t, err)
}

func TestWaitForQueryStatusPreservesCallerCancellationDuringRead(t *testing.T) {
	var reads atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	server := queryStatusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) == 1 {
			writeQueryStatusResponse(w, tfe.QueryRunRunning)
			return
		}
		cancel()
		<-r.Context().Done()
	})
	tfeClient := queryStatusTFEClient(t, server)

	response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", time.Millisecond, nil)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(2), reads.Load())
}

func TestWaitForQueryStatusPreservesCallerCancellationAfterSuccessfulRead(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	callerErr := errors.New("MCP request canceled")
	tfeClient := queryStatusStubClient(func(context.Context, string) (*tfe.QueryRun, error) {
		cancel(callerErr)
		return &tfe.QueryRun{ID: "qry-test", Status: tfe.QueryRunFinished}, nil
	})

	response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", time.Second, nil)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, callerErr)
}

func TestWaitForQueryStatusPreservesAPIErrorRacingInternalBudget(t *testing.T) {
	apiErr := errors.New("query run forbidden")
	ctx, cancel := context.WithCancelCause(context.Background())
	reads := 0
	tfeClient := queryStatusStubClient(func(context.Context, string) (*tfe.QueryRun, error) {
		reads++
		if reads == 1 {
			return &tfe.QueryRun{ID: "qry-test", Status: tfe.QueryRunRunning}, nil
		}
		cancel(errQueryStatusWaitBudgetExceeded)
		return nil, apiErr
	})

	response, err := waitForQueryStatus(context.Background(), ctx, tfeClient, "qry-test", time.Millisecond, nil)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, apiErr)
}

func TestWaitForQueryStatusRejectsInvalidPollingConfiguration(t *testing.T) {
	tfeClient := &tfe.Client{}

	ctx := context.Background()
	response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", 0, nil)
	assert.Nil(t, response)
	assert.EqualError(t, err, "poll interval must be greater than zero")
}

func TestGetQueryStatusHandlerReturnsStructuredAndTextResults(t *testing.T) {
	server := queryStatusTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeQueryStatusResponse(w, tfe.QueryRunFinished)
	})
	ctx := queryStatusHandlerContext(t, server)
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "qry-test"},
	}}

	result, err := getQueryStatusHandlerWithConfig(ctx, request, silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	structured, ok := result.StructuredContent.(*queryStatusResponse)
	require.True(t, ok)
	assert.Equal(t, "qry-test", structured.ID)
	assert.Equal(t, tfe.QueryRunFinished, structured.Status)
	assert.True(t, structured.Terminal)
	assert.Zero(t, structured.RetryAfterSeconds)
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, text.Text, "get_query_summary")
	assert.Contains(t, text.Text, `"query_run_id":"qry-test"`)
	assert.Contains(t, text.Text, `"terminal":true`)
	assert.NotContains(t, text.Text, `"retry_after_seconds"`)
}

func TestGetQueryStatusHandlerReturnsNonterminalStatusAsSuccess(t *testing.T) {
	server := queryStatusTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeQueryStatusResponse(w, tfe.QueryRunRunning)
	})
	ctx := queryStatusHandlerContext(t, server)
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "qry-test"},
	}}

	result, err := getQueryStatusHandlerWithConfig(ctx, request, silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	structured, ok := result.StructuredContent.(*queryStatusResponse)
	require.True(t, ok)
	assert.Equal(t, "qry-test", structured.ID)
	assert.Equal(t, tfe.QueryRunRunning, structured.Status)
	assert.False(t, structured.Terminal)
	assert.Equal(t, 5, structured.RetryAfterSeconds)
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, text.Text, "get_query_status again with the same query_run_id after 5 seconds")
	assert.Contains(t, text.Text, `"query_run_id":"qry-test"`)
	assert.Contains(t, text.Text, `"retry_after_seconds":5`)
}

func TestGetQueryStatusHandlerReturnsAPIFailureAsToolError(t *testing.T) {
	server := queryStatusTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"Query run not found"}]}`))
	})
	ctx := queryStatusHandlerContext(t, server)
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "qry-missing"},
	}}

	result, err := getQueryStatusHandlerWithConfig(ctx, request, silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, result.StructuredContent)
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, text.Text, "failed to get query run")
}

func TestGetQueryStatusHandlerRejectsMissingQueryRunID(t *testing.T) {
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "  "},
	}}

	result, err := getQueryStatusHandlerWithConfig(context.Background(), request, silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestGetQueryStatusHandlerPreservesCallerCancellation(t *testing.T) {
	server := queryStatusTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeQueryStatusResponse(w, tfe.QueryRunRunning)
	})
	ctx := queryStatusHandlerContext(t, server)
	ctx, cancel := context.WithCancelCause(ctx)
	callerErr := errors.New("MCP request canceled")
	cancel(callerErr)
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "qry-test"},
	}}

	result, err := getQueryStatusHandlerWithConfig(ctx, request, silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, text.Text, callerErr.Error())
}

func TestWaitForQueryStatusLogsPollingAndCancellation(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New()
	logger.SetOutput(&buf)
	logger.SetLevel(log.DebugLevel)
	ctx, cancel := context.WithCancel(context.Background())
	reads := 0
	tfeClient := queryStatusStubClient(func(context.Context, string) (*tfe.QueryRun, error) {
		reads++
		if reads == 1 {
			return &tfe.QueryRun{ID: "qry-test", Status: tfe.QueryRunRunning}, nil
		}
		cancel()
		return nil, context.Canceled
	})

	response, err := waitForQueryStatus(ctx, ctx, tfeClient, "qry-test", time.Millisecond, logger)

	assert.Nil(t, response)
	assert.ErrorIs(t, err, context.Canceled)
	output := buf.String()
	assert.Contains(t, output, "query_run_id=qry-test")
	assert.Contains(t, output, "poll_count=2")
	assert.Contains(t, output, "last_status=running")
	assert.Contains(t, output, "reason=caller_canceled")
	assert.Contains(t, output, "elapsed=")
	assert.NotContains(t, output, "test-token")
}

func TestReadQueryStatus(t *testing.T) {
	type requestDetails struct {
		method        string
		path          string
		authorization string
	}
	requests := make(chan requestDetails, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}

		requests <- requestDetails{method: r.Method, path: r.URL.Path, authorization: r.Header.Get("Authorization")}
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
	request := <-requests
	assert.Equal(t, http.MethodGet, request.method)
	assert.Equal(t, "/api/v2/queries/qry-test", request.path)
	assert.Equal(t, "Bearer test-token", request.authorization)
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

func queryStatusTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func queryStatusTFEClient(t *testing.T, testServer *httptest.Server) *tfe.Client {
	t.Helper()
	tfeClient, err := tfe.NewClient(&tfe.Config{Address: testServer.URL, Token: "test-token", HTTPClient: testServer.Client()})
	require.NoError(t, err)
	return tfeClient
}

func queryStatusHandlerContext(t *testing.T, testServer *httptest.Server) context.Context {
	t.Helper()
	sessionID := "get-query-status-" + t.Name()
	t.Setenv(client.TerraformToken, "test-token")
	_, err := client.NewTfeClient(sessionID, testServer.URL, false, "test-token", "", silentLogger())
	require.NoError(t, err)
	t.Cleanup(func() { client.DeleteTfeClient(sessionID) })
	mcpServer := server.NewMCPServer("test", "test")
	return mcpServer.WithContext(context.Background(), queryStatusTestSession{id: sessionID})
}

func testQueryStatusPollConfig() queryStatusPollConfig {
	return queryStatusPollConfig{
		pollInterval: time.Millisecond,
		waitBudget:   10 * time.Millisecond,
	}
}

func writeQueryStatusResponse(w http.ResponseWriter, status tfe.QueryRunStatus) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":%q,"terraform-version":"1.14.0","generate-config-out":false}}}`, status)
}

func queryStatusStubClient(read func(context.Context, string) (*tfe.QueryRun, error)) *tfe.Client {
	return &tfe.Client{QueryRuns: queryRunsStub{read: read}}
}

var _ server.ClientSession = queryStatusTestSession{}
