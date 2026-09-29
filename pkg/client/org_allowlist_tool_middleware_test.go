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
			arguments:   map[string]any{OrgNameArgument: "allowed-org"},
			wantAllowed: true,
		},
		{
			name:        "rejects organization not in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{OrgNameArgument: "blocked-org"},
			wantAllowed: false,
		},
		{
			name:        "accepts normalized Search alias",
			allowlist:   []string{" Allowed-Org "},
			arguments:   map[string]any{"organization_name": "ALLOWED-ORG"},
			wantAllowed: true,
		},
		{
			name:      "rejects conflicting aliases",
			allowlist: []string{"allowed-org"},
			arguments: map[string]any{OrgNameArgument: "allowed-org", "organization_name": "blocked-org"},
		},
		{
			name:      "rejects nonstring alias",
			allowlist: []string{"allowed-org"},
			arguments: map[string]any{"organization_name": true},
		},
		{
			name:        "empty policy permits any backend-authorized organization",
			arguments:   map[string]any{"organization_name": "example"},
			wantAllowed: true,
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

func TestOrganizationAllowlistResolvedTarget(t *testing.T) {
	for _, org := range []string{"allowed", "other", ""} {
		t.Run(org, func(t *testing.T) {
			handler := OrganizationAllowlistToolMiddleware([]string{"allowed"}, nil)(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				// Handle-only handlers must resolve their stored target, then check
				// both this deployment policy and current backend authorization.
				if err := AuthorizeOrganization(ctx, org); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText("allowed"), nil
			})
			result, err := handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"attempt_id": "opaque"}}})
			assert.NoError(t, err)
			assert.Equal(t, org != "allowed", result.IsError)
		})
	}
}
