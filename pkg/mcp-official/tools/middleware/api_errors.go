// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
)

// Error codes for tool errors from HCPT, TFE or the registry. Clients can check
// these instead of parsing the error text.
const (
	APIErrorUnauthorized = "unauthorized"
	APIErrorNotFound     = "not_found"
	APIErrorConflict     = "conflict"
	APIErrorRateLimited  = "rate_limited"
	APIErrorUnavailable  = "unavailable"
)

// Keys for the error info in a tool result's _meta.
const (
	MetaErrorCode  = "error_code"
	MetaHTTPStatus = "http_status"
)

// apiError is the code and HTTP status for an API error we can identify.
type apiError struct {
	Code   string
	Status int
}

// conflictErrors are the go-tfe errors for a 409. go-tfe only returns these
// for workspace lock, unlock and safe delete calls.
var conflictErrors = []error{
	tfe.ErrWorkspaceLocked,
	tfe.ErrWorkspaceNotLocked,
	tfe.ErrWorkspaceLockedByRun,
	tfe.ErrWorkspaceLockedByTeam,
	tfe.ErrWorkspaceLockedByUser,
	tfe.ErrWorkspaceLockedCannotDelete,
	tfe.ErrWorkspaceStillProcessing,
	tfe.ErrWorkspaceNotSafeToDelete,
}

// classifyAPIError maps the go-tfe or registry error from a failed API call to
// a code and the status the API sent back. Anything we can't tell for sure, like a network
// error or a 400 with a custom message, returns false so we don't guess.
//
// HCPT sends a 404 for things the token can't see, so not_found can also mean
// the token just doesn't have access.
func classifyAPIError(err error) (apiError, bool) {
	switch {
	case err == nil:
		return apiError{}, false
	case errors.Is(err, tfe.ErrUnauthorized):
		return apiError{Code: APIErrorUnauthorized, Status: http.StatusUnauthorized}, true
	case errors.Is(err, tfe.ErrResourceNotFound):
		return apiError{Code: APIErrorNotFound, Status: http.StatusNotFound}, true
	}
	for _, conflict := range conflictErrors {
		if errors.Is(err, conflict) {
			return apiError{Code: APIErrorConflict, Status: http.StatusConflict}, true
		}
	}

	var registryErr *tfeclient.RegistryError
	if errors.As(err, &registryErr) {
		return classifyRegistryStatus(registryErr.StatusCode)
	}
	return apiError{}, false
}

// classifyRegistryStatus maps a status from the public registry to a code.
// Other statuses, like a 400, return false since we can't say much about them.
func classifyRegistryStatus(status int) (apiError, bool) {
	switch {
	case status == http.StatusNotFound:
		return apiError{Code: APIErrorNotFound, Status: status}, true
	case status == http.StatusTooManyRequests:
		return apiError{Code: APIErrorRateLimited, Status: status}, true
	case status >= http.StatusInternalServerError:
		return apiError{Code: APIErrorUnavailable, Status: status}, true
	}
	return apiError{}, false
}

// APIErrors labels tool errors that came from an API response we recognize. It
// puts the code at the front of the error text, like "unauthorized: ...", and
// adds the code and status to _meta, so clients can tell an expired token from
// a missing resource without reading the text.
//
// It uses the error the SDK keeps on the result (GetError), so tools just need
// to wrap the go-tfe error with %w.
func APIErrors() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if method != "tools/call" || err != nil {
				return result, err
			}

			toolResult, ok := result.(*mcp.CallToolResult)
			if !ok || toolResult == nil || !toolResult.IsError {
				return result, err
			}
			apiErr, ok := classifyAPIError(toolResult.GetError())
			if !ok {
				return result, err
			}

			if len(toolResult.Content) > 0 {
				if text, ok := toolResult.Content[0].(*mcp.TextContent); ok {
					text.Text = apiErr.Code + ": " + text.Text
				}
			}
			if toolResult.Meta == nil {
				toolResult.Meta = mcp.Meta{}
			}
			toolResult.Meta[MetaErrorCode] = apiErr.Code
			toolResult.Meta[MetaHTTPStatus] = apiErr.Status
			return result, err
		}
	}
}
