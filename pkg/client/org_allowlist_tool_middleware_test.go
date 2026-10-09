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
			name:        "allows Search organization in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{SearchOrgNameArgument: "allowed-org"},
			wantAllowed: true,
		},
		{
			name:        "rejects Search organization not in allowlist",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{SearchOrgNameArgument: "blocked-org"},
			wantAllowed: false,
		},
		{
			name:        "normalizes Search organization and allowlist",
			allowlist:   []string{" Allowed-Org "},
			arguments:   map[string]any{SearchOrgNameArgument: "  ALLOWED-org "},
			wantAllowed: true,
		},
		{
			name:        "allows matching aliases",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{OrgNameArgument: "allowed-org", SearchOrgNameArgument: "Allowed-Org"},
			wantAllowed: true,
		},
		{
			name:        "rejects conflicting aliases even when one is allowed",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{OrgNameArgument: "allowed-org", SearchOrgNameArgument: "blocked-org"},
			wantAllowed: false,
		},
		{
			name:        "rejects non-string Search organization",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{SearchOrgNameArgument: 42},
			wantAllowed: false,
		},
		{
			name:        "rejects non-string terraform organization",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{OrgNameArgument: []string{"allowed-org"}},
			wantAllowed: false,
		},
		{
			name:        "ignores blank alias beside an allowed one",
			allowlist:   []string{"allowed-org"},
			arguments:   map[string]any{OrgNameArgument: "  ", SearchOrgNameArgument: "allowed-org"},
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

func TestOrganizationAllowlistToolMiddlewarePropagatesPolicy(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	var authorizeErr error
	var configured bool
	handler := OrganizationAllowlistToolMiddleware([]string{"Org-A"}, logger)(
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			configured = OrganizationAllowlistConfigured(ctx)
			authorizeErr = AuthorizeOrganization(ctx, "org-b")
			assert.NoError(t, AuthorizeOrganization(ctx, " ORG-A "))
			assert.ErrorIs(t, AuthorizeOrganization(ctx, ""), ErrOrganizationNotAllowed)
			return mcp.NewToolResultText("ok"), nil
		})

	_, err := handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "get_query_status",
		Arguments: map[string]any{"query_run_id": "qry-1"},
	}})

	assert.NoError(t, err)
	assert.True(t, configured)
	assert.ErrorIs(t, authorizeErr, ErrOrganizationNotAllowed)
}

func TestAuthorizeOrganizationWithoutAllowlist(t *testing.T) {
	assert.False(t, OrganizationAllowlistConfigured(context.Background()))
	assert.NoError(t, AuthorizeOrganization(context.Background(), "anything"))
}
