// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
)

const (
	// DefaultBaseURL is the default HCP Terraform API base URL
	DefaultBaseURL = "https://app.terraform.io/api/v2"
	// DefaultTimeout is the default HTTP client timeout
	DefaultTimeout = 30 * time.Second
	// DefaultRetryMax is the default number of retries
	DefaultRetryMax = 3
)

// Client handles authentication and API calls to HCP Terraform
type Client struct {
	httpClient *http.Client
	logger     *log.Logger
	baseURL    string
}

// NewClient creates a new HCP Terraform client
func NewClient(logger *log.Logger) *Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = logger

	transport := cleanhttp.DefaultPooledTransport()
	transport.Proxy = http.ProxyFromEnvironment

	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.HTTPClient.Timeout = DefaultTimeout
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = DefaultRetryMax

	// Custom backoff function that respects rate limiting headers
	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Check for Retry-After header
			if retryAfterStr := resp.Header.Get("Retry-After"); retryAfterStr != "" {
				if seconds, err := strconv.Atoi(retryAfterStr); err == nil {
					return time.Duration(seconds) * time.Second
				}
			}
			// Check for x-ratelimit-reset header
			if resetStr := resp.Header.Get("x-ratelimit-reset"); resetStr != "" {
				if resetTime, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					resetAfter := time.Until(time.Unix(resetTime, 0))
					if resetAfter > 0 {
						return resetAfter
					}
				}
			}
		}
		// Default exponential backoff
		return retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
	}

	return &Client{
		httpClient: retryClient.StandardClient(),
		logger:     logger,
		baseURL:    DefaultBaseURL,
	}
}

// GetOrganizations fetches organizations from HCP Terraform
func (c *Client) GetOrganizations(token string, opts *OrganizationListOptions) (*OrganizationResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("%s/organizations", c.baseURL)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("invalid endpoint URL: %v", err))
	}

	// Add query parameters
	if opts != nil {
		query := reqURL.Query()
		if opts.Query != "" {
			query.Set("q", opts.Query)
		}
		if opts.QueryEmail != "" {
			query.Set("q[email]", opts.QueryEmail)
		}
		if opts.QueryName != "" {
			query.Set("q[name]", opts.QueryName)
		}
		if opts.PageNumber > 0 {
			query.Set("page[number]", strconv.Itoa(opts.PageNumber))
		}
		if opts.PageSize > 0 {
			query.Set("page[size]", strconv.Itoa(opts.PageSize))
		}
		reqURL.RawQuery = query.Encode()
	}

	c.logger.Debugf("Requesting HCP Terraform organizations from: %s", reqURL.String())

	// Create HTTP request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, NewNetworkError("failed to create HTTP request", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("User-Agent", "terraform-mcp-server")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, NewNetworkError("HTTP request failed", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewNetworkError("failed to read response body", err)
	}

	c.logger.Debugf("Response status: %s", resp.Status)
	c.logger.Tracef("Response body: %s", string(body))

	// Parse JSON response
	var orgResponse OrganizationResponse
	if err := json.Unmarshal(body, &orgResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Fetched %d organizations", len(orgResponse.Data))
	return &orgResponse, nil
}

// ValidateToken checks if the provided token is valid by making a simple API call
func (c *Client) ValidateToken(token string) error {
	if token == "" {
		return NewAuthenticationError("token cannot be empty")
	}

	// Try to fetch organizations with minimal parameters to validate token
	opts := &OrganizationListOptions{
		PageSize: 1, // Minimal request
	}

	_, err := c.GetOrganizations(token, opts)
	if err != nil {
		// If it's an authentication or authorization error, return as-is
		if hcpErr, ok := err.(*HCPTerraformError); ok {
			if hcpErr.Type == ErrorTypeAuthentication || hcpErr.Type == ErrorTypeAuthorization {
				return err
			}
		}
		// For other errors, wrap them as validation errors
		return NewValidationError(fmt.Sprintf("token validation failed: %v", err))
	}

	c.logger.Debugf("Token validation successful")
	return nil
}

// SetBaseURL allows customizing the base URL (useful for testing or enterprise installations)
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// SetTimeout sets the HTTP client timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}
