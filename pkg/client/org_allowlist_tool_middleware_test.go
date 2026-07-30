// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestOrganizationAllowlistToolMiddleware(t *testing.T) {
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
			arguments:   map[string]any{orgNameArgument: "allowed-org"},
			wantAllowed: true,
		},
		{
			name:        "rejects organization not in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{orgNameArgument: "blocked-org"},
			wantAllowed: false,
		},
	}

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false

			mockHandler := func(
				context.Context,
				mcp.CallToolRequest,
			) (*mcp.CallToolResult, error) {
				nextCalled = true
				return mcp.NewToolResultText("success"), nil
			}

			handler := OrganizationAllowlistToolMiddleware(
				test.allowlist,
				logger,
			)(mockHandler)

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "test_tool",
					Arguments: test.arguments,
				},
			}

			result, err := handler(context.Background(), request)
			assert.NoError(t, err)
			assert.Equal(t, test.wantAllowed, nextCalled)
			assert.Equal(t, !test.wantAllowed, result.IsError)
		})
	}
}
