// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// hasSessionRateLimiter peeks at RateLimitMiddleware's private per-session
// limiter map, taking the same private mutex the middleware itself uses
// (also read via reflection) so this doesn't race with concurrent
// Allow/DeleteSession calls. It's only used to prove EndSessionHandler
// actually released the per-session bucket - the same reflect+unsafe
// technique already used by getServerHooksForTest in main_metrics_test.go.
func hasSessionRateLimiter(t *testing.T, rl *client.RateLimitMiddleware, sessionID string) bool {
	t.Helper()

	elem := reflect.ValueOf(rl).Elem()

	muValue := elem.FieldByName("mu")
	require.True(t, muValue.IsValid())
	mu := (*sync.RWMutex)(unsafe.Pointer(muValue.UnsafeAddr()))
	mu.RLock()
	defer mu.RUnlock()

	value := elem.FieldByName("sessionLimiters")
	require.True(t, value.IsValid())

	mapPtr := reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
	sessionLimiters, ok := mapPtr.Interface().(map[string]*rate.Limiter)
	require.True(t, ok)

	_, exists := sessionLimiters[sessionID]
	return exists
}

// TestGetOfficialStreamableServer_SessionLifecycle proves the WithOnSession
// wiring added in getOfficialStreamableServer replaces the mark3labs
// OnRegisterSession/OnUnregisterSession hooks: connecting a client should
// pre-warm its per-session HTTP client, and closing the client should
// release it (and its rate-limit bucket) once the server observes the
// disconnect.
func TestGetOfficialStreamableServer_SessionLifecycle(t *testing.T) {
	logger := metricsTestLogger()
	rateLimiter := client.NewRateLimitMiddleware(client.DefaultRateLimitConfig(), logger)

	handler := getOfficialStreamableServer(
		context.Background(),
		0,     // heartbeatInterval
		false, // isStateless
		client.CORSConfig{Mode: "disabled"},
		logger,
		nil, // organizationAllowlist
		[]string{"list_workspaces"},
		rateLimiter,
		client.MetricsConfig{Enabled: false},
	)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL + "/official"}

	clientSession, err := mcpClient.Connect(context.Background(), transport, nil)
	require.NoError(t, err)

	sessionID := clientSession.ID()
	require.NotEmpty(t, sessionID)

	// WithOnSession's NewSessionHandler call runs synchronously inside the
	// "notifications/initialized" handler, so the session's HTTP client
	// should already be cached by the time Connect returns. Poll briefly
	// anyway to avoid flakiness against the async transport plumbing.
	require.Eventually(t, func() bool {
		return client.GetHttpClient(sessionID) != nil
	}, time.Second, 10*time.Millisecond, "expected a session HTTP client to be pre-warmed on session init")

	// Seed a per-session rate-limit bucket so we can also prove it gets
	// cleaned up alongside the cached clients.
	require.NoError(t, rateLimiter.Allow(sessionID, "list_workspaces"))
	assert.True(t, hasSessionRateLimiter(t, rateLimiter, sessionID))

	require.NoError(t, clientSession.Close())

	// EndSessionHandler runs in the background goroutine started by
	// WithOnSession, unblocked by session.Wait() once the connection closes.
	require.Eventually(t, func() bool {
		return client.GetHttpClient(sessionID) == nil
	}, time.Second, 10*time.Millisecond, "expected the session HTTP client to be released after disconnect")

	assert.False(t, hasSessionRateLimiter(t, rateLimiter, sessionID), "expected the per-session rate limit bucket to be released after disconnect")
}
