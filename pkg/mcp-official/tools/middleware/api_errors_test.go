// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func errorResult(err error) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}
	result.SetError(err)
	return result
}

func TestAPIErrors(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_run_details"}}

	tests := []struct {
		name       string
		result     *mcp.CallToolResult
		wantText   string
		wantCode   any
		wantStatus any
	}{
		{
			name:       "unauthorized",
			result:     errorResult(fmt.Errorf("failed to read run run-123: %w", tfe.ErrUnauthorized)),
			wantText:   "unauthorized: failed to read run run-123: unauthorized",
			wantCode:   APIErrorUnauthorized,
			wantStatus: 401,
		},
		{
			name:       "not found",
			result:     errorResult(fmt.Errorf("failed to read run run-123: %w", tfe.ErrResourceNotFound)),
			wantText:   "not_found: failed to read run run-123: resource not found",
			wantCode:   APIErrorNotFound,
			wantStatus: 404,
		},
		{
			name:     "unknown error is left alone",
			result:   errorResult(errors.New("connection refused")),
			wantText: "connection refused",
		},
		{
			name:     "success is left alone",
			result:   &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}},
			wantText: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return tt.result, nil
			}

			result, err := APIErrors()(next)(context.Background(), "tools/call", request)
			require.NoError(t, err)

			toolResult := result.(*mcp.CallToolResult)
			require.Len(t, toolResult.Content, 1)
			assert.Equal(t, tt.wantText, toolResult.Content[0].(*mcp.TextContent).Text)

			if tt.wantCode == nil {
				assert.Empty(t, toolResult.Meta)
				return
			}
			assert.Equal(t, tt.wantCode, toolResult.Meta[MetaErrorCode])
			assert.Equal(t, tt.wantStatus, toolResult.Meta[MetaHTTPStatus])
		})
	}
}

func TestAPIErrorsIgnoresOtherMethods(t *testing.T) {
	called := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.ListToolsResult{}, nil
	}

	result, err := APIErrors()(next)(context.Background(), "tools/list", &mcp.ListToolsRequest{})
	require.NoError(t, err)
	assert.True(t, called)
	assert.IsType(t, &mcp.ListToolsResult{}, result)
}

func TestAPIErrorsKeepsExistingMeta(t *testing.T) {
	result := errorResult(fmt.Errorf("reading: %w", tfe.ErrUnauthorized))
	result.Meta = mcp.Meta{"other": "value"}
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return result, nil
	}

	_, err := APIErrors()(next)(context.Background(), "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "x"}})
	require.NoError(t, err)
	assert.Equal(t, "value", result.Meta["other"])
	assert.Equal(t, APIErrorUnauthorized, result.Meta[MetaErrorCode])
}

func TestClassifyAPIError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   apiError
		wantOK bool
	}{
		{name: "nil", err: nil},
		{name: "unauthorized", err: tfe.ErrUnauthorized, want: apiError{Code: APIErrorUnauthorized, Status: http.StatusUnauthorized}, wantOK: true},
		{name: "wrapped unauthorized", err: fmt.Errorf("listing workspaces: %w", tfe.ErrUnauthorized), want: apiError{Code: APIErrorUnauthorized, Status: http.StatusUnauthorized}, wantOK: true},
		{name: "not found", err: tfe.ErrResourceNotFound, want: apiError{Code: APIErrorNotFound, Status: http.StatusNotFound}, wantOK: true},
		{name: "wrapped not found", err: fmt.Errorf("reading run: %w", tfe.ErrResourceNotFound), want: apiError{Code: APIErrorNotFound, Status: http.StatusNotFound}, wantOK: true},
		{name: "workspace locked", err: tfe.ErrWorkspaceLocked, want: apiError{Code: APIErrorConflict, Status: http.StatusConflict}, wantOK: true},
		{name: "locked by run", err: fmt.Errorf("unlocking: %w", tfe.ErrWorkspaceLockedByRun), want: apiError{Code: APIErrorConflict, Status: http.StatusConflict}, wantOK: true},
		{name: "not safe to delete", err: tfe.ErrWorkspaceNotSafeToDelete, want: apiError{Code: APIErrorConflict, Status: http.StatusConflict}, wantOK: true},
		{name: "plain error is not guessed", err: errors.New("invalid attribute")},
		{name: "network error is not guessed", err: context.DeadlineExceeded},
		{name: "error text alone is not enough", err: errors.New("unauthorized")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := classifyAPIError(tt.err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
