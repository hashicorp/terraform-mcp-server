// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"

	log "github.com/sirupsen/logrus"
)

// SecurityEventMessage is the log message for every security event.
const SecurityEventMessage = "security event"

// SecurityEventComponent is the component field on every security event. The
// secops pipeline matches on it and the event field.
const SecurityEventComponent = "mcp-security"

// Security event types, logged in the "event" field.
const (
	// SecurityEventMissingToken is a request without a bearer token when one is required
	SecurityEventMissingToken = "missing_token"
	// SecurityEventInvalidToken is a token HCP Terraform rejected with a 401
	SecurityEventInvalidToken = "invalid_token"
	// SecurityEventOrganizationNotAllowed is a request or tool call blocked by the org allowlist
	SecurityEventOrganizationNotAllowed = "organization_not_allowed"
	// SecurityEventRateLimited is a tool call rejected by the server's rate limiter
	SecurityEventRateLimited = "rate_limited"
)

// SecurityEvent is one security event. Tool, Organization and Reason are left
// out of the log when empty. The token is never logged.
type SecurityEvent struct {
	Event        string
	Tool         string
	Organization string
	Reason       string
}

// securityClientIPKey holds the client IP for logging. It's always set by
// TerraformContextMiddleware, unlike ClientIPKey which is only set when the IP
// gets forwarded to HCP Terraform.
const securityClientIPKey contextKey = "security_client_ip"

// securityEventFields builds the fields for an event, with the request id and
// client IP taken from ctx.
func securityEventFields(ctx context.Context, event SecurityEvent) map[string]any {
	fields := map[string]any{"component": SecurityEventComponent, "event": event.Event}
	add := func(key, value string) {
		if value != "" {
			fields[key] = value
		}
	}
	add("request_id", RequestIDFromContext(ctx))
	clientIP, _ := ctx.Value(securityClientIPKey).(string)
	add("client_ip", clientIP)
	add("tool", event.Tool)
	add("organization", event.Organization)
	add("reason", event.Reason)
	return fields
}

// LogSecurityEvent logs event as a warning with structured fields.
func LogSecurityEvent(ctx context.Context, logger *log.Logger, event SecurityEvent) {
	if logger == nil {
		return
	}
	logger.WithFields(securityEventFields(ctx, event)).Warn(SecurityEventMessage)
}
