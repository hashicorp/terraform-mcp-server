// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

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

	handler := ToolLoggingMiddleware(logger)(next)
	_, err = handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	assert.True(t, nextCalled)

	logOutput := buf.String()
	t.Logf("Captured log output:\n%s", logOutput)
	assert.Contains(t, logOutput, "level=INFO")
	assert.Contains(t, logOutput, "list_workspaces")
	assert.Contains(t, logOutput, "terraform_org_name")
	assert.Contains(t, logOutput, "my-org")
}

func TestToolLoggingMiddleware_IgnoresNonToolCalls(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.ListToolsResult{}, nil
	}

	handler := ToolLoggingMiddleware(logger)(next)
	_, err := handler(context.Background(), "tools/list", &mcp.ListToolsRequest{})
	require.NoError(t, err)

	assert.True(t, nextCalled)
	assert.Empty(t, buf.String(), "non-tools/call methods should not be logged")
}
