// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationAllowlistMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		allowlist   []string
		arguments   map[string]any
		wantAllowed bool
	}{
		{
			name:        "allows tool without organization argument",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{},
			wantAllowed: true,
		},
		{
			name:        "allows organization in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{"terraform_org_name": "allowed-org"},
			wantAllowed: true,
		},
		{
			name:        "rejects organization not in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{"terraform_org_name": "blocked-org"},
			wantAllowed: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false
			next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				nextCalled = true
				return &mcp.CallToolResult{}, nil
			}

			argsJSON, err := json.Marshal(test.arguments)
			require.NoError(t, err)

			request := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Name:      "test_tool",
					Arguments: argsJSON,
				},
			}

			handler := OrganizationAllowlistMiddleware(test.allowlist, logger)(next)
			result, err := handler(context.Background(), "tools/call", request)
			require.NoError(t, err)

			assert.Equal(t, test.wantAllowed, nextCalled)

			callToolResult, ok := result.(*mcp.CallToolResult)
			require.True(t, ok)
			assert.Equal(t, !test.wantAllowed, callToolResult.IsError)
		})
	}
}

func TestOrganizationAllowlistMiddleware_IgnoresNonToolCalls(t *testing.T) {
	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.ListToolsResult{}, nil
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := OrganizationAllowlistMiddleware([]string{"allowed-org"}, logger)(next)
	_, err := handler(context.Background(), "tools/list", &mcp.ListToolsRequest{})

	require.NoError(t, err)
	assert.True(t, nextCalled, "non-tools/call methods should always pass through")
}
