// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.CallToolResult{}, nil
	}

	argsJSON, err := json.Marshal(map[string]any{"terraform_org_name": "my-org"})
	require.NoError(t, err)

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "list_workspaces",
			Arguments: argsJSON,
		},
	}

	handler := ToolLogging(logger)(next)
	_, err = handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	assert.True(t, nextCalled)

	logOutput := buf.String()
	t.Logf("Captured log output:\n%s", logOutput)
	assert.Contains(t, logOutput, "level=INFO")
	assert.Contains(t, logOutput, "msg=\"tool call completed\"")
	assert.Contains(t, logOutput, "list_workspaces")
	assert.Contains(t, logOutput, "terraform_org_name")
	assert.NotContains(t, logOutput, "my-org", "argument values should not be logged")
}

func TestToolLoggingMiddleware_RequestID(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "list_workspaces"}}

	t.Run("logs the ID from the context and passes it on", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		var nextID string
		next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			nextID = tfeclient.RequestIDFromContext(ctx)
			return &mcp.CallToolResult{}, nil
		}

		ctx := tfeclient.WithRequestID(context.Background(), "req-123")
		_, err := ToolLogging(logger)(next)(ctx, "tools/call", request)
		require.NoError(t, err)
		assert.Equal(t, "req-123", nextID)
		assert.Contains(t, buf.String(), "request_id=req-123")
	})

	t.Run("adds an ID when there isn't one, like in stdio mode", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		var nextID string
		next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			nextID = tfeclient.RequestIDFromContext(ctx)
			return &mcp.CallToolResult{}, nil
		}

		_, err := ToolLogging(logger)(next)(context.Background(), "tools/call", request)
		require.NoError(t, err)
		require.NotEmpty(t, nextID)
		assert.Contains(t, buf.String(), "request_id="+nextID)
	})
}

func TestToolLoggingMiddleware_DoesNotLogArgumentValues(t *testing.T) {
	const secret = "super-secret-value"

	argsJSON, err := json.Marshal(map[string]any{
		"terraform_org_name": "my-org",
		"workspace_name":     "my-workspace",
		"variable_key":       "db_password",
		"variable_value":     secret,
		"sensitive":          true,
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		next mcp.MethodHandler
	}{
		{
			name: "success",
			next: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return &mcp.CallToolResult{}, nil
			},
		},
		{
			name: "tool error",
			next: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				result := &mcp.CallToolResult{}
				result.SetError(errors.New("creating variable: conflict"))
				return result, nil
			},
		},
		{
			name: "method error",
			next: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return nil, errors.New("method handler unavailable")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "create_workspace_variable",
					Arguments: argsJSON,
				},
			}

			_, _ = ToolLogging(logger)(tt.next)(context.Background(), "tools/call", request)

			logOutput := buf.String()
			assert.Contains(t, logOutput, "create_workspace_variable")
			assert.Contains(t, logOutput, "variable_value", "argument keys should still be logged")
			assert.NotContains(t, logOutput, secret, "argument values must not be logged")
			assert.NotContains(t, logOutput, "db_password")
			assert.NotContains(t, logOutput, "my-workspace")
		})
	}
}

func TestToolLoggingMiddleware_ToolError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	toolErr := errors.New("listing workspaces: permission denied")
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		result := &mcp.CallToolResult{}
		result.SetError(toolErr)
		return result, nil
	}

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "list_workspaces"},
	}

	handler := ToolLogging(logger)(next)
	result, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	assert.True(t, result.(*mcp.CallToolResult).IsError)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "level=ERROR")
	assert.Contains(t, logOutput, "msg=\"tool call failed\"")
	assert.Contains(t, logOutput, "list_workspaces")
	assert.Contains(t, logOutput, "listing workspaces: permission denied")
}

func TestToolLoggingMiddleware_MethodError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	methodErr := errors.New("method handler unavailable")
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, methodErr
	}

	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "list_workspaces"},
	}

	handler := ToolLogging(logger)(next)
	_, err := handler(context.Background(), "tools/call", request)
	require.ErrorIs(t, err, methodErr)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "level=ERROR")
	assert.Contains(t, logOutput, "msg=\"tool call failed\"")
	assert.Contains(t, logOutput, "list_workspaces")
	assert.Contains(t, logOutput, "method handler unavailable")
}

func TestToolLoggingMiddleware_IgnoresNonToolCalls(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.ListToolsResult{}, nil
	}

	handler := ToolLogging(logger)(next)
	_, err := handler(context.Background(), "tools/list", &mcp.ListToolsRequest{})
	require.NoError(t, err)

	assert.True(t, nextCalled)
	assert.Empty(t, buf.String(), "non-tools/call methods should not be logged")
}

func TestToolLoggingMiddleware_RedactsArgumentErrors(t *testing.T) {
	const secret = "9.87654321e+08"

	tests := []struct {
		name           string
		next           mcp.MethodHandler
		expectedPrefix string
	}{
		{
			name: "schema validation error",
			next: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf(`validating "arguments": validating root: validating /properties/variable_value: type: %s has type "number", want "string"`, secret))
				return result, nil
			},
			expectedPrefix: `validating \"arguments\": details redacted`,
		},
		{
			name: "unmarshaling error",
			next: func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return nil, fmt.Errorf("unmarshaling arguments: bad value %s", secret)
			},
			expectedPrefix: "unmarshaling arguments: details redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "create_workspace_variable"},
			}

			_, _ = ToolLogging(logger)(tt.next)(context.Background(), "tools/call", request)

			logOutput := buf.String()
			assert.Contains(t, logOutput, tt.expectedPrefix)
			assert.NotContains(t, logOutput, secret, "the rejected value must not be logged")
		})
	}
}

func TestLoggableError(t *testing.T) {
	assert.Equal(t, "", loggableError(nil))
	assert.Equal(t, "listing workspaces: permission denied", loggableError(errors.New("listing workspaces: permission denied")))
	assert.Equal(t, `validating "arguments": details redacted`, loggableError(errors.New(`validating "arguments": type: 123 has type "integer"`)))
	assert.Equal(t, "unmarshaling arguments: details redacted", loggableError(errors.New("calling tool: unmarshaling arguments: secret")))
}

func TestArgumentKeys(t *testing.T) {
	tests := []struct {
		name     string
		raw      json.RawMessage
		expected []string
	}{
		{name: "nil arguments", raw: nil, expected: []string{}},
		{name: "empty object", raw: json.RawMessage(`{}`), expected: []string{}},
		{name: "malformed json", raw: json.RawMessage(`{not json`), expected: []string{}},
		{name: "not an object", raw: json.RawMessage(`["a","b"]`), expected: []string{}},
		{
			name:     "keys sorted, values dropped",
			raw:      json.RawMessage(`{"workspace_name":"ws","terraform_org_name":"org","variable_value":"secret"}`),
			expected: []string{"terraform_org_name", "variable_value", "workspace_name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, argumentKeys(tt.raw))
		})
	}
}
