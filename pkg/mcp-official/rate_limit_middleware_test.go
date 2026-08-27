// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRateLimitMiddleware(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // reduce noise in test output

	config := tfeclient.RateLimitConfig{
		GlobalLimit:     rate.Every(time.Second), // 1 request per second
		GlobalBurst:     1,
		PerSessionLimit: rate.Every(time.Second),
		PerSessionBurst: 1,
	}
	rl := tfeclient.NewRateLimitMiddleware(config, logger)

	nextCalled := 0
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled++
		return &mcp.CallToolResult{}, nil
	}

	argsJSON, err := json.Marshal(map[string]any{})
	require.NoError(t, err)
	request := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test_tool",
			Arguments: argsJSON,
		},
	}

	handler := RateLimitMiddleware(rl)(next)

	// First call is within the burst of 1, so it should go through.
	result, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	callToolResult, ok := result.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, callToolResult.IsError)
	assert.Equal(t, 1, nextCalled)

	// Second call, immediately after, exceeds the global burst of 1.
	result, err = handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	callToolResult, ok = result.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.True(t, callToolResult.IsError)
	assert.Equal(t, 1, nextCalled, "next should not be called once the limit is hit")
}

func TestRateLimitMiddleware_IgnoresNonToolCalls(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Burst of 0 means even a single tools/call would be rejected, so this
	// proves the method-mismatch check runs before any rate-limit check.
	config := tfeclient.RateLimitConfig{
		GlobalLimit: rate.Every(time.Second),
		GlobalBurst: 0,
	}
	rl := tfeclient.NewRateLimitMiddleware(config, logger)

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.ListToolsResult{}, nil
	}

	handler := RateLimitMiddleware(rl)(next)
	_, err := handler(context.Background(), "tools/list", &mcp.ListToolsRequest{})

	require.NoError(t, err)
	assert.True(t, nextCalled, "non-tools/call methods should always pass through")
}
