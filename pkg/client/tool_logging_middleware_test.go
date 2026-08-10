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
	t.Run("logs tool call name and args at info level", func(t *testing.T) {
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
		assert.Contains(t, logOutput, "my-org")
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
