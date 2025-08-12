// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Error types for better error handling
type ErrorType string

const (
	ErrorTypeAuthentication ErrorType = "authentication"
	ErrorTypeAuthorization  ErrorType = "authorization"
	ErrorTypeRateLimit      ErrorType = "rate_limit"
	ErrorTypeNotFound       ErrorType = "not_found"
	ErrorTypeValidation     ErrorType = "validation"
	ErrorTypeNetwork        ErrorType = "network"
	ErrorTypeUnknown        ErrorType = "unknown"
)

// HCPTerraformError represents errors from HCP Terraform API
type HCPTerraformError struct {
	Type       ErrorType
	Message    string
	StatusCode int
	RetryAfter *time.Duration // For rate limiting
	Err        error
}

func (e *HCPTerraformError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("HCP Terraform API error (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HCP Terraform error: %s", e.Message)
}

func (e *HCPTerraformError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error indicates a retryable condition
func (e *HCPTerraformError) IsRetryable() bool {
	return e.Type == ErrorTypeRateLimit || e.Type == ErrorTypeNetwork
}

// NewErrorFromResponse creates an appropriate error from HTTP response
func NewErrorFromResponse(resp *http.Response, err error) *HCPTerraformError {
	hcpErr := &HCPTerraformError{
		StatusCode: resp.StatusCode,
		Err:        err,
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		hcpErr.Type = ErrorTypeAuthentication
		hcpErr.Message = "Invalid or missing authentication token"
	case http.StatusForbidden:
		hcpErr.Type = ErrorTypeAuthorization
		hcpErr.Message = "Insufficient permissions for this operation"
	case http.StatusNotFound:
		hcpErr.Type = ErrorTypeNotFound
		hcpErr.Message = "Resource not found"
	case http.StatusUnprocessableEntity:
		hcpErr.Type = ErrorTypeValidation
		hcpErr.Message = "Validation error in request"
	case http.StatusTooManyRequests:
		hcpErr.Type = ErrorTypeRateLimit
		hcpErr.Message = "Rate limit exceeded"
		// Extract retry-after header if present
		if retryAfterStr := resp.Header.Get("Retry-After"); retryAfterStr != "" {
			if seconds, parseErr := strconv.Atoi(retryAfterStr); parseErr == nil {
				retryAfter := time.Duration(seconds) * time.Second
				hcpErr.RetryAfter = &retryAfter
			}
		}
		// Also check for x-ratelimit-reset header (used by some APIs)
		if resetStr := resp.Header.Get("x-ratelimit-reset"); resetStr != "" {
			if resetTime, parseErr := strconv.ParseInt(resetStr, 10, 64); parseErr == nil {
				resetAfter := time.Until(time.Unix(resetTime, 0))
				if resetAfter > 0 {
					hcpErr.RetryAfter = &resetAfter
				}
			}
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		hcpErr.Type = ErrorTypeNetwork
		hcpErr.Message = "Server error - request may be retryable"
	default:
		hcpErr.Type = ErrorTypeUnknown
		hcpErr.Message = fmt.Sprintf("Unexpected HTTP status: %d", resp.StatusCode)
	}

	return hcpErr
}

// NewAuthenticationError creates an authentication error
func NewAuthenticationError(message string) *HCPTerraformError {
	return &HCPTerraformError{
		Type:    ErrorTypeAuthentication,
		Message: message,
	}
}

// NewValidationError creates a validation error
func NewValidationError(message string) *HCPTerraformError {
	return &HCPTerraformError{
		Type:    ErrorTypeValidation,
		Message: message,
	}
}

// NewNetworkError creates a network error
func NewNetworkError(message string, err error) *HCPTerraformError {
	return &HCPTerraformError{
		Type:    ErrorTypeNetwork,
		Message: message,
		Err:     err,
	}
}
