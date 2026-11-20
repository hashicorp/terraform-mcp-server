// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func securityTestLogger() (*log.Logger, *test.Hook) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger, test.NewLocal(logger)
}

// securityEvents returns the security events the hook saw, as their fields.
func securityEvents(hook *test.Hook) []log.Fields {
	var events []log.Fields
	for _, entry := range hook.AllEntries() {
		if entry.Message == SecurityEventMessage {
			events = append(events, entry.Data)
		}
	}
	return events
}

func TestSecurityEventFields(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-1")
	ctx = context.WithValue(ctx, securityClientIPKey, "203.0.113.9")

	t.Run("includes request id and client ip from ctx", func(t *testing.T) {
		fields := securityEventFields(ctx, SecurityEvent{Event: SecurityEventRateLimited, Tool: "list_runs", Reason: "global rate limit exceeded"})
		assert.Equal(t, map[string]any{
			"component":  SecurityEventComponent,
			"event":      SecurityEventRateLimited,
			"request_id": "req-1",
			"client_ip":  "203.0.113.9",
			"tool":       "list_runs",
			"reason":     "global rate limit exceeded",
		}, fields)
	})

	t.Run("leaves out empty fields", func(t *testing.T) {
		fields := securityEventFields(context.Background(), SecurityEvent{Event: SecurityEventMissingToken})
		assert.Equal(t, map[string]any{"component": SecurityEventComponent, "event": SecurityEventMissingToken}, fields)
	})

	t.Run("nil logger is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { LogSecurityEvent(ctx, nil, SecurityEvent{Event: SecurityEventMissingToken}) })
	})
}

// serveThroughContext runs a request through TerraformContextMiddleware first,
// the same order the server uses, so the request id and client IP are set.
func serveThroughContext(t *testing.T, logger *log.Logger, inner func(http.Handler) http.Handler, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := TerraformContextMiddleware(logger)(inner(next))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.RemoteAddr = "203.0.113.9:51234"
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestRequireTokenLogsSecurityEvents(t *testing.T) {
	t.Setenv(RequireTokenEnv, "true")

	t.Run("missing token", func(t *testing.T) {
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, RequireTerraformTokenMiddleware(logger), "")
		require.Equal(t, http.StatusUnauthorized, rr.Code)

		events := securityEvents(hook)
		require.Len(t, events, 1)
		assert.Equal(t, SecurityEventMissingToken, events[0]["event"])
		assert.Equal(t, "203.0.113.9", events[0]["client_ip"])
		assert.Equal(t, rr.Header().Get(RequestIDHeader), events[0]["request_id"], "the event should carry the same id the client got back")
	})

	t.Run("invalid token", func(t *testing.T) {
		useFakeOrganizationLister(t, &fakeOrganizationLister{err: tfe.ErrUnauthorized})
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, RequireTerraformTokenMiddleware(logger), "Bearer junk-token")
		require.Equal(t, http.StatusUnauthorized, rr.Code)

		events := securityEvents(hook)
		require.Len(t, events, 1)
		assert.Equal(t, SecurityEventInvalidToken, events[0]["event"])
		assert.Equal(t, "203.0.113.9", events[0]["client_ip"])
		for _, entry := range hook.AllEntries() {
			assert.NotContains(t, entry.Message, "junk-token")
			for _, value := range entry.Data {
				assert.NotEqual(t, "junk-token", value, "the token must never be logged")
			}
		}
	})

	t.Run("valid token logs nothing", func(t *testing.T) {
		useFakeOrganizationLister(t, &fakeOrganizationLister{})
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, RequireTerraformTokenMiddleware(logger), "Bearer good-token")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Empty(t, securityEvents(hook))
	})
}

func TestOrganizationAllowlistLogsSecurityEvents(t *testing.T) {
	allowlist := []string{"acme"}

	t.Run("missing token", func(t *testing.T) {
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, OrganizationAllowlistMiddleware(allowlist, logger), "")
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		events := securityEvents(hook)
		require.Len(t, events, 1)
		assert.Equal(t, SecurityEventMissingToken, events[0]["event"])
	})

	t.Run("invalid token", func(t *testing.T) {
		useFakeOrganizationLister(t, &fakeOrganizationLister{err: tfe.ErrUnauthorized})
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, OrganizationAllowlistMiddleware(allowlist, logger), "Bearer junk")
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		events := securityEvents(hook)
		require.Len(t, events, 1)
		assert.Equal(t, SecurityEventInvalidToken, events[0]["event"])
	})

	t.Run("token without an allowed org", func(t *testing.T) {
		useFakeOrganizationLister(t, &fakeOrganizationLister{pages: []*tfe.OrganizationList{{
			Items: []*tfe.Organization{{Name: "other-org"}},
		}}})
		logger, hook := securityTestLogger()
		rr := serveThroughContext(t, logger, OrganizationAllowlistMiddleware(allowlist, logger), "Bearer someone-else")
		require.Equal(t, http.StatusForbidden, rr.Code)
		events := securityEvents(hook)
		require.Len(t, events, 1)
		assert.Equal(t, SecurityEventOrganizationNotAllowed, events[0]["event"])
		assert.Equal(t, "203.0.113.9", events[0]["client_ip"])
	})
}

func TestClientIPIsLoggedWithoutForwarding(t *testing.T) {
	t.Setenv(ForwardClientIP, "")
	var securityIP, forwardedIP any
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		securityIP = r.Context().Value(securityClientIPKey)
		forwardedIP = r.Context().Value(contextKey(ClientIPKey))
	})
	logger, _ := securityTestLogger()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.RemoteAddr = "203.0.113.9:51234"
	TerraformContextMiddleware(logger)(next).ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.9", securityIP, "security events always get the client ip")
	assert.Nil(t, forwardedIP, "but it's only forwarded to HCPT when MCP_FORWARD_CLIENT_IP is on")
}

func TestRateLimitLogsSecurityEvent(t *testing.T) {
	logger, hook := securityTestLogger()
	limiter := NewRateLimitMiddleware(RateLimitConfig{GlobalLimit: 1, GlobalBurst: 1, PerSessionLimit: 100, PerSessionBurst: 100}, logger)
	ctx := WithRequestID(context.Background(), "req-rl")
	ctx = context.WithValue(ctx, securityClientIPKey, "203.0.113.9")

	require.NoError(t, limiter.Allow(ctx, "session-1", "list_runs"))
	require.Error(t, limiter.Allow(ctx, "session-1", "list_runs"))

	events := securityEvents(hook)
	require.Len(t, events, 1)
	assert.Equal(t, SecurityEventRateLimited, events[0]["event"])
	assert.Equal(t, "list_runs", events[0]["tool"])
	assert.Equal(t, "req-rl", events[0]["request_id"])
	assert.Equal(t, "global rate limit exceeded", events[0]["reason"])
	assert.NotContains(t, events[0], "session_id", "session ids shouldn't end up in secops logs")
}

func TestLegacyToolAllowlistLogsSecurityEvent(t *testing.T) {
	logger, hook := securityTestLogger()
	handler := OrganizationAllowlistToolMiddleware([]string{"acme"}, logger)(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Name:      "list_workspaces",
		Arguments: map[string]any{OrgNameArgument: "Other-Org"},
	}}

	result, err := handler(WithRequestID(context.Background(), "req-legacy"), request)
	require.NoError(t, err)
	require.True(t, result.IsError)

	events := securityEvents(hook)
	require.Len(t, events, 1)
	assert.Equal(t, SecurityEventOrganizationNotAllowed, events[0]["event"])
	assert.Equal(t, "list_workspaces", events[0]["tool"])
	assert.Equal(t, "other-org", events[0]["organization"])
	assert.Equal(t, "req-legacy", events[0]["request_id"])
}
