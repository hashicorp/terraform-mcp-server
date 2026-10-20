// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolLoggingMiddleware(t *testing.T) {
	t.Run("logs tool call name and argument keys at info level", func(t *testing.T) {
		var buf bytes.Buffer
		logger := log.New()
		logger.SetOutput(&buf)
		logger.SetLevel(log.InfoLevel)

		nextCalled := false
		mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			nextCalled = true
			return mcp.NewToolResultText("success"), nil
		}

		handler := ToolLoggingMiddleware(logger)(mockHandler)

		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "list_workspaces",
				Arguments: map[string]any{
					"terraform_org_name": "my-org",
				},
			},
		}

		result, err := handler(context.Background(), request)

		require.NoError(t, err)
		assert.True(t, nextCalled)
		assert.False(t, result.IsError)

		logOutput := buf.String()
		t.Logf("Captured log output:\n%s", logOutput)
		assert.Contains(t, logOutput, "level=info")
		assert.Contains(t, logOutput, "list_workspaces")
		assert.Contains(t, logOutput, "terraform_org_name")
		assert.NotContains(t, logOutput, "my-org", "argument values should not be logged")
	})

	t.Run("does not log sensitive argument values", func(t *testing.T) {
		const secret = "super-secret-value"

		var buf bytes.Buffer
		logger := log.New()
		logger.SetOutput(&buf)
		logger.SetLevel(log.InfoLevel)

		mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("success"), nil
		}

		handler := ToolLoggingMiddleware(logger)(mockHandler)

		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "create_workspace_variable",
				Arguments: map[string]any{
					"terraform_org_name": "my-org",
					"workspace_name":     "my-workspace",
					"variable_key":       "db_password",
					"variable_value":     secret,
					"sensitive":          true,
				},
			},
		}

		_, err := handler(context.Background(), request)
		require.NoError(t, err)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "create_workspace_variable")
		assert.Contains(t, logOutput, "variable_value", "argument keys should still be logged")
		assert.NotContains(t, logOutput, secret, "argument values must not be logged")
		assert.NotContains(t, logOutput, "db_password")
		assert.NotContains(t, logOutput, "my-workspace")
	})

	t.Run("handles nil logger gracefully", func(t *testing.T) {
		nextCalled := false
		mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			nextCalled = true
			return mcp.NewToolResultText("success"), nil
		}

		handler := ToolLoggingMiddleware(nil)(mockHandler)

		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "test_tool",
			},
		}

		result, err := handler(context.Background(), request)

		require.NoError(t, err)
		assert.True(t, nextCalled)
		assert.False(t, result.IsError)
	})

	t.Run("does not log Search arguments", func(t *testing.T) {
		var buf bytes.Buffer
		logger := log.New()
		logger.SetOutput(&buf)
		logger.SetLevel(log.InfoLevel)
		handler := ToolLoggingMiddleware(logger)(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("success"), nil
		})
		request := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "prepare_import", Arguments: map[string]any{
			"import_context": "search-log-secret-sentinel",
		}}}

		_, err := handler(context.Background(), request)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "prepare_import")
		assert.NotContains(t, buf.String(), "search-log-secret-sentinel")
		assert.NotContains(t, buf.String(), "import_context")
	})

	t.Run("passes through tool handler errors and results", func(t *testing.T) {
		logger := log.New()
		logger.SetLevel(log.ErrorLevel) // Suppress log output during error test

		expectedErr := errors.New("tool error")
		mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, expectedErr
		}

		handler := ToolLoggingMiddleware(logger)(mockHandler)

		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "failing_tool",
			},
		}

		result, err := handler(context.Background(), request)

		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})
}

func TestToolArgumentKeys(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		expected []string
	}{
		{name: "nil arguments", args: nil, expected: []string{}},
		{name: "empty arguments", args: map[string]any{}, expected: []string{}},
		{
			name:     "keys sorted, values dropped",
			args:     map[string]any{"workspace_name": "ws", "terraform_org_name": "org", "variable_value": "secret"},
			expected: []string{"terraform_org_name", "variable_value", "workspace_name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, toolArgumentKeys(tt.args))
		})
	}
}
