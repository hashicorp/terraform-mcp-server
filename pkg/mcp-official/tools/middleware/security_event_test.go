// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
)

func securityHook() (*log.Logger, *test.Hook) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger, test.NewLocal(logger)
}

func securityEntries(hook *test.Hook) []log.Fields {
	var events []log.Fields
	for _, entry := range hook.AllEntries() {
		if entry.Message == tfeclient.SecurityEventMessage {
			events = append(events, entry.Data)
		}
	}
	return events
}

func TestOfficialAllowlistLogsSecurityEvent(t *testing.T) {
	logger, hook := securityHook()
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:      "list_workspaces",
		Arguments: json.RawMessage(`{"terraform_org_name":"Other-Org"}`),
	}}
	ctx := tfeclient.WithRequestID(context.Background(), "req-official")

	result, err := OrganizationAllowlist([]string{"acme"}, logger)(next)(ctx, "tools/call", request)
	require.NoError(t, err)
	require.True(t, result.(*mcp.CallToolResult).IsError)

	events := securityEntries(hook)
	require.Len(t, events, 1)
	assert.Equal(t, tfeclient.SecurityEventComponent, events[0]["component"])
	assert.Equal(t, tfeclient.SecurityEventOrganizationNotAllowed, events[0]["event"])
	assert.Equal(t, "list_workspaces", events[0]["tool"])
	assert.Equal(t, "other-org", events[0]["organization"])
	assert.Equal(t, "req-official", events[0]["request_id"])
}

func TestAPIErrorsLogsInvalidToken(t *testing.T) {
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "get_run_details"}}

	t.Run("unauthorized logs an event", func(t *testing.T) {
		logger, hook := securityHook()
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return errorResult(fmt.Errorf("failed to read run: %w", tfe.ErrUnauthorized)), nil
		}
		_, err := APIErrors(logger)(next)(tfeclient.WithRequestID(context.Background(), "req-401"), "tools/call", request)
		require.NoError(t, err)

		events := securityEntries(hook)
		require.Len(t, events, 1)
		assert.Equal(t, tfeclient.SecurityEventComponent, events[0]["component"])
		assert.Equal(t, tfeclient.SecurityEventInvalidToken, events[0]["event"])
		assert.Equal(t, "get_run_details", events[0]["tool"])
		assert.Equal(t, "req-401", events[0]["request_id"])
	})

	t.Run("other api errors don't", func(t *testing.T) {
		logger, hook := securityHook()
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return errorResult(fmt.Errorf("failed to read run: %w", tfe.ErrResourceNotFound)), nil
		}
		_, err := APIErrors(logger)(next)(context.Background(), "tools/call", request)
		require.NoError(t, err)
		assert.Empty(t, securityEntries(hook))
	})
}
