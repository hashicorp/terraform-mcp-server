// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const (
	// RequestIDHeader carries the correlation ID on calls to HCP Terraform, TFE and
	// the registry, and on the response back to the MCP client.
	RequestIDHeader = "X-Request-Id"

	// RequestIDKey is the context key the correlation ID is stored under.
	RequestIDKey = "REQUEST_ID"
)

// NewRequestID returns a new random correlation ID.
func NewRequestID() string {
	return uuid.NewString()
}

// WithRequestID returns a copy of ctx that carries the given correlation ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey(RequestIDKey), id)
}

// RequestIDFromContext returns the correlation ID in ctx, or "" if there isn't one.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey(RequestIDKey)).(string)
	return id
}

// EnsureRequestID returns the request id in ctx, or adds a new one if there
// isn't one. In http mode TerraformContextMiddleware already set it. stdio has
// no http request, so the tool logging middleware calls this to give each tool
// call its own id.
func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if id := RequestIDFromContext(ctx); id != "" {
		return ctx, id
	}
	id := NewRequestID()
	return WithRequestID(ctx, id), id
}

// requestIDTransport sets X-Request-Id on outgoing requests when ctx has a
// request id. It reads the id on every request instead of setting it once when
// the client gets built, since tfe and http clients are cached per session and
// reused.
type requestIDTransport struct {
	base http.RoundTripper
}

func (t *requestIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := RequestIDFromContext(req.Context())
	if id == "" || req.Header.Get(RequestIDHeader) != "" {
		return t.base.RoundTrip(req)
	}
	// RoundTrippers shouldn't change the caller's request
	req = req.Clone(req.Context())
	req.Header.Set(RequestIDHeader, id)
	return t.base.RoundTrip(req)
}
