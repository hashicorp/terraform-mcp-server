// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// ====================
// Workspace Methods
// ====================

// GetWorkspaces fetches workspaces from HCP Terraform
func (c *Client) GetWorkspaces(token string, organizationName string, opts *WorkspaceListOptions) (*WorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if organizationName == "" {
		return nil, NewValidationError("organization name is required")
	}

	// Build URL with query parameters
	endpoint := fmt.Sprintf("%s/organizations/%s/workspaces", c.baseURL, organizationName)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("invalid endpoint URL: %v", err))
	}

	// Add query parameters
	if opts != nil {
		query := reqURL.Query()
		if opts.PageNumber > 0 {
			query.Set("page[number]", strconv.Itoa(opts.PageNumber))
		}
		if opts.PageSize > 0 {
			query.Set("page[size]", strconv.Itoa(opts.PageSize))
		}
		if opts.SearchName != "" {
			query.Set("search[name]", opts.SearchName)
		}
		if len(opts.SearchTags) > 0 {
			query.Set("search[tags]", strings.Join(opts.SearchTags, ","))
		}
		if len(opts.SearchExcludeTags) > 0 {
			query.Set("search[exclude-tags]", strings.Join(opts.SearchExcludeTags, ","))
		}
		if opts.SearchWildcardName != "" {
			query.Set("search[wildcard-name]", opts.SearchWildcardName)
		}
		if opts.Sort != "" {
			query.Set("sort", opts.Sort)
		}
		if opts.FilterProjectID != "" {
			query.Set("filter[project][id]", opts.FilterProjectID)
		}
		if opts.FilterCurrentRunStatus != "" {
			query.Set("filter[current-run][status]", opts.FilterCurrentRunStatus)
		}
		// Handle tag filters
		for i, key := range opts.FilterTaggedKeys {
			query.Set(fmt.Sprintf("filter[tagged][%d][key]", i), key)
		}
		for i, value := range opts.FilterTaggedValues {
			query.Set(fmt.Sprintf("filter[tagged][%d][value]", i), value)
		}
		reqURL.RawQuery = query.Encode()
	}

	c.logger.Debugf("Requesting HCP Terraform workspaces from: %s", reqURL.String())

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
	var workspaceResponse WorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Fetched %d workspaces", len(workspaceResponse.Data))
	return &workspaceResponse, nil
}

// GetWorkspaceByID fetches a workspace by its ID
func (c *Client) GetWorkspaceByID(token string, workspaceID string) (*SingleWorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s", c.baseURL, workspaceID)

	c.logger.Debugf("Requesting HCP Terraform workspace from: %s", endpoint)

	// Create HTTP request
	req, err := http.NewRequest("GET", endpoint, nil)
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
	var workspaceResponse SingleWorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Fetched workspace: %s", workspaceResponse.Data.Attributes.Name)
	return &workspaceResponse, nil
}

// GetWorkspaceByName fetches a workspace by organization and workspace name
func (c *Client) GetWorkspaceByName(token string, organizationName, workspaceName string) (*SingleWorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if organizationName == "" {
		return nil, NewValidationError("organization name is required")
	}
	if workspaceName == "" {
		return nil, NewValidationError("workspace name is required")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/organizations/%s/workspaces/%s", c.baseURL, organizationName, workspaceName)

	c.logger.Debugf("Requesting HCP Terraform workspace from: %s", endpoint)

	// Create HTTP request
	req, err := http.NewRequest("GET", endpoint, nil)
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
	var workspaceResponse SingleWorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Fetched workspace: %s", workspaceResponse.Data.Attributes.Name)
	return &workspaceResponse, nil
}

// CreateWorkspace creates a new workspace
func (c *Client) CreateWorkspace(token string, organizationName string, request *WorkspaceCreateRequest) (*SingleWorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if organizationName == "" {
		return nil, NewValidationError("organization name is required")
	}
	if request == nil {
		return nil, NewValidationError("workspace create request is required")
	}

	// Validate required fields
	if request.Data.Attributes.Name == "" {
		return nil, NewValidationError("workspace name is required")
	}
	if request.Data.Type != "workspaces" {
		return nil, NewValidationError("request type must be 'workspaces'")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/organizations/%s/workspaces", c.baseURL, organizationName)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Creating HCP Terraform workspace at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
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
	if resp.StatusCode != http.StatusCreated {
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
	var workspaceResponse SingleWorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Created workspace: %s", workspaceResponse.Data.Attributes.Name)
	return &workspaceResponse, nil
}

// UpdateWorkspaceByID updates a workspace by its ID
func (c *Client) UpdateWorkspaceByID(token string, workspaceID string, request *WorkspaceUpdateRequest) (*SingleWorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("workspace update request is required")
	}

	// Validate required fields
	if request.Data.Type != "workspaces" {
		return nil, NewValidationError("request type must be 'workspaces'")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s", c.baseURL, workspaceID)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Updating HCP Terraform workspace at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("PATCH", endpoint, bytes.NewBuffer(requestBody))
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
	var workspaceResponse SingleWorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Updated workspace: %s", workspaceResponse.Data.Attributes.Name)
	return &workspaceResponse, nil
}

// UpdateWorkspaceByName updates a workspace by organization and workspace name
func (c *Client) UpdateWorkspaceByName(token string, organizationName, workspaceName string, request *WorkspaceUpdateRequest) (*SingleWorkspaceResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if organizationName == "" {
		return nil, NewValidationError("organization name is required")
	}
	if workspaceName == "" {
		return nil, NewValidationError("workspace name is required")
	}
	if request == nil {
		return nil, NewValidationError("workspace update request is required")
	}

	// Validate required fields
	if request.Data.Type != "workspaces" {
		return nil, NewValidationError("request type must be 'workspaces'")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/organizations/%s/workspaces/%s", c.baseURL, organizationName, workspaceName)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Updating HCP Terraform workspace at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("PATCH", endpoint, bytes.NewBuffer(requestBody))
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
	var workspaceResponse SingleWorkspaceResponse
	if err := json.Unmarshal(body, &workspaceResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Debugf("Updated workspace: %s", workspaceResponse.Data.Attributes.Name)
	return &workspaceResponse, nil
}

// ====================
// Variable Methods
// ====================

// GetWorkspaceVariables retrieves all variables for a workspace
func (c *Client) GetWorkspaceVariables(token string, workspaceID string) (*VariableResponse, error) {
	if token == "" {
		return nil, NewValidationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/vars", c.baseURL, workspaceID)

	c.logger.Debugf("Requesting HCP Terraform variables from: %s", endpoint)

	// Create HTTP request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, NewNetworkError("failed to create HTTP request", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("User-Agent", "terraform-mcp-server")

	// Send request with retry logic
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
	var variableResponse VariableResponse
	if err := json.Unmarshal(body, &variableResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Retrieved %d variables for workspace %s", len(variableResponse.Data), workspaceID)
	return &variableResponse, nil
}

// CreateWorkspaceVariable creates a new variable in a workspace
func (c *Client) CreateWorkspaceVariable(token string, workspaceID string, request *VariableCreateRequest) (*SingleVariableResponse, error) {
	if token == "" {
		return nil, NewValidationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("variable create request is required")
	}

	// Validate required fields
	if request.Data.Attributes.Key == "" {
		return nil, NewValidationError("variable key is required")
	}
	if request.Data.Attributes.Category == "" {
		return nil, NewValidationError("variable category is required")
	}
	if request.Data.Attributes.Category != "terraform" && request.Data.Attributes.Category != "env" {
		return nil, NewValidationError("variable category must be 'terraform' or 'env'")
	}

	// Set the type
	request.Data.Type = "vars"

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/vars", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Creating variable in workspace %s at: %s", workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
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
	if resp.StatusCode != http.StatusCreated {
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
	var variableResponse SingleVariableResponse
	if err := json.Unmarshal(body, &variableResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Created variable: %s", variableResponse.Data.Attributes.Key)
	return &variableResponse, nil
}

// UpdateWorkspaceVariable updates an existing variable in a workspace
func (c *Client) UpdateWorkspaceVariable(token string, workspaceID string, variableID string, request *VariableUpdateRequest) (*SingleVariableResponse, error) {
	if token == "" {
		return nil, NewValidationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if variableID == "" {
		return nil, NewValidationError("variable ID is required")
	}
	if request == nil {
		return nil, NewValidationError("variable update request is required")
	}

	// Set required fields
	request.Data.ID = variableID
	request.Data.Type = "vars"

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/vars/%s", c.baseURL, workspaceID, variableID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Updating variable %s in workspace %s at: %s", variableID, workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("PATCH", endpoint, bytes.NewBuffer(requestBody))
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
	var variableResponse SingleVariableResponse
	if err := json.Unmarshal(body, &variableResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Updated variable: %s", variableResponse.Data.Attributes.Key)
	return &variableResponse, nil
}

// DeleteWorkspaceVariable deletes a variable from a workspace
func (c *Client) DeleteWorkspaceVariable(token string, workspaceID string, variableID string) error {
	if token == "" {
		return NewValidationError("authentication token is required")
	}
	if workspaceID == "" {
		return NewValidationError("workspace ID is required")
	}
	if variableID == "" {
		return NewValidationError("variable ID is required")
	}

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/vars/%s", c.baseURL, workspaceID, variableID)

	c.logger.Debugf("Deleting variable %s from workspace %s at: %s", variableID, workspaceID, endpoint)

	// Create HTTP request
	req, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return NewNetworkError("failed to create HTTP request", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("User-Agent", "terraform-mcp-server")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NewNetworkError("HTTP request failed", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusNoContent {
		return NewErrorFromResponse(resp, nil)
	}

	c.logger.Infof("Deleted variable %s from workspace %s", variableID, workspaceID)
	return nil
}

// BulkCreateWorkspaceVariables creates multiple variables in a workspace at once
func (c *Client) BulkCreateWorkspaceVariables(token string, workspaceID string, variables []VariableCreateData) (*BulkVariableCreateResponse, error) {
	if token == "" {
		return nil, NewValidationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if len(variables) == 0 {
		return nil, NewValidationError("at least one variable is required")
	}

	// Prepare bulk request
	request := &BulkVariableCreateRequest{
		Data: variables,
	}

	// Validate and set types for all variables
	for i := range request.Data {
		if request.Data[i].Attributes.Key == "" {
			return nil, NewValidationError(fmt.Sprintf("variable %d: key is required", i))
		}
		if request.Data[i].Attributes.Category == "" {
			return nil, NewValidationError(fmt.Sprintf("variable %d: category is required", i))
		}
		if request.Data[i].Attributes.Category != "terraform" && request.Data[i].Attributes.Category != "env" {
			return nil, NewValidationError(fmt.Sprintf("variable %d: category must be 'terraform' or 'env'", i))
		}
		request.Data[i].Type = "vars"
	}

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/vars", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Creating %d variables in workspace %s at: %s", len(variables), workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create HTTP request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
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
	if resp.StatusCode != http.StatusCreated {
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
	var bulkResponse BulkVariableCreateResponse
	if err := json.Unmarshal(body, &bulkResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Created %d variables in workspace %s", len(bulkResponse.Data), workspaceID)
	return &bulkResponse, nil
}

// GetWorkspaceConfigurationVersions retrieves configuration versions for a workspace
func (c *Client) GetWorkspaceConfigurationVersions(token, workspaceID string, pageNumber, pageSize int) (*ConfigurationVersionsResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build URL with pagination
	endpoint := fmt.Sprintf("%s/workspaces/%s/configuration-versions", c.baseURL, workspaceID)
	if pageNumber > 0 || pageSize > 0 {
		params := make([]string, 0)
		if pageNumber > 0 {
			params = append(params, fmt.Sprintf("page[number]=%d", pageNumber))
		}
		if pageSize > 0 {
			params = append(params, fmt.Sprintf("page[size]=%d", pageSize))
		}
		if len(params) > 0 {
			endpoint += "?" + strings.Join(params, "&")
		}
	}

	c.logger.Debugf("Getting configuration versions from: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Configuration versions response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var configVersions ConfigurationVersionsResponse
	if err := json.Unmarshal(body, &configVersions); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Retrieved %d configuration versions for workspace %s", len(configVersions.Data), workspaceID)
	return &configVersions, nil
}

// CreateWorkspaceConfigurationVersion creates a new configuration version for a workspace
func (c *Client) CreateWorkspaceConfigurationVersion(token, workspaceID string, request *ConfigurationVersionCreateRequest) (*ConfigurationVersionResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("configuration version create request is required")
	}

	// Validate required fields
	if request.Data.Type != "configuration-versions" {
		return nil, NewValidationError("request type must be 'configuration-versions'")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/configuration-versions", c.baseURL, workspaceID)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Creating configuration version at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Create configuration version response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var configVersion ConfigurationVersionResponse
	if err := json.Unmarshal(body, &configVersion); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Created configuration version %s for workspace %s", configVersion.Data.ID, workspaceID)
	return &configVersion, nil
}

// DownloadConfigurationVersion downloads configuration files from a configuration version
func (c *Client) DownloadConfigurationVersion(token, configurationVersionID string) ([]byte, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if configurationVersionID == "" {
		return nil, NewValidationError("configuration version ID is required")
	}

	// First get the configuration version to extract the download URL
	endpoint := fmt.Sprintf("%s/configuration-versions/%s", c.baseURL, configurationVersionID)

	c.logger.Debugf("Getting configuration version details from: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var configVersion ConfigurationVersionResponse
	if err := json.Unmarshal(body, &configVersion); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	// Check if download link is available
	if configVersion.Data.Links == nil || configVersion.Data.Links.Download == nil {
		return nil, NewValidationError("configuration version does not have a download link available")
	}

	downloadURL := *configVersion.Data.Links.Download
	c.logger.Debugf("Downloading configuration files from: %s", downloadURL)

	// Download the configuration files
	downloadReq, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}

	// Set headers for download
	downloadReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Make download request
	downloadResp, err := c.httpClient.Do(downloadReq)
	if err != nil {
		return nil, err
	}
	defer downloadResp.Body.Close()

	// Check download status code
	if downloadResp.StatusCode < 200 || downloadResp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(downloadResp, nil)
	}

	// Read configuration files content
	configFiles, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Infof("Downloaded configuration files for configuration version %s (%d bytes)", configurationVersionID, len(configFiles))
	return configFiles, nil
}

// UploadConfigurationFiles uploads configuration files to a configuration version
func (c *Client) UploadConfigurationFiles(uploadURL string, configurationFiles []byte) error {
	if uploadURL == "" {
		return NewValidationError("upload URL is required")
	}
	if len(configurationFiles) == 0 {
		return NewValidationError("configuration files content is required")
	}

	c.logger.Debugf("Uploading configuration files to: %s", uploadURL)

	// Create upload request
	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(configurationFiles))
	if err != nil {
		return err
	}

	// Set headers for upload (Content-Type should be application/octet-stream for tar.gz)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(configurationFiles))

	// Make upload request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check upload status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewErrorFromResponse(resp, nil)
	}

	c.logger.Infof("Successfully uploaded configuration files (%d bytes)", len(configurationFiles))
	return nil
}

// GetCurrentStateVersion retrieves the current state version for a workspace
func (c *Client) GetCurrentStateVersion(token, workspaceID string) (*StateVersionResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/current-state-version", c.baseURL, workspaceID)

	c.logger.Debugf("Getting current state version from: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Current state version response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var stateVersion StateVersionResponse
	if err := json.Unmarshal(body, &stateVersion); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Retrieved current state version %s for workspace %s", stateVersion.Data.ID, workspaceID)
	return &stateVersion, nil
}

// DownloadStateVersion downloads state data from a state version
func (c *Client) DownloadStateVersion(token, stateVersionID string) ([]byte, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if stateVersionID == "" {
		return nil, NewValidationError("state version ID is required")
	}

	// First get the state version to extract the download URL
	endpoint := fmt.Sprintf("%s/state-versions/%s", c.baseURL, stateVersionID)

	c.logger.Debugf("Getting state version details from: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var stateVersion StateVersionResponse
	if err := json.Unmarshal(body, &stateVersion); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	// Check if download link is available
	downloadURL := stateVersion.Data.Attributes.DownloadURL
	if downloadURL == nil || *downloadURL == "" {
		return nil, NewValidationError("state version does not have a download link available")
	}

	c.logger.Debugf("Downloading state data from: %s", *downloadURL)

	// Download the state data
	downloadReq, err := http.NewRequest("GET", *downloadURL, nil)
	if err != nil {
		return nil, err
	}

	// Set headers for download
	downloadReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Make download request
	downloadResp, err := c.httpClient.Do(downloadReq)
	if err != nil {
		return nil, err
	}
	defer downloadResp.Body.Close()

	// Check download status code
	if downloadResp.StatusCode < 200 || downloadResp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(downloadResp, nil)
	}

	// Read state data content
	stateData, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Infof("Downloaded state data for state version %s (%d bytes)", stateVersionID, len(stateData))
	return stateData, nil
}

// CreateStateVersion creates a new state version for a workspace
func (c *Client) CreateStateVersion(token, workspaceID string, request *StateVersionCreateRequest) (*StateVersionResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("state version create request is required")
	}

	// Validate required fields
	if request.Data.Type != "state-versions" {
		return nil, NewValidationError("request type must be 'state-versions'")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/state-versions", c.baseURL, workspaceID)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Creating state version at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Create state version response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var stateVersion StateVersionResponse
	if err := json.Unmarshal(body, &stateVersion); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Created state version %s for workspace %s", stateVersion.Data.ID, workspaceID)
	return &stateVersion, nil
}

// GetWorkspaceTagBindings retrieves tag bindings for a workspace
func (c *Client) GetWorkspaceTagBindings(token, workspaceID string) (*TagBindingsResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/tag-bindings", c.baseURL, workspaceID)

	c.logger.Debugf("Getting workspace tag bindings from: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Workspace tag bindings response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var tagBindings TagBindingsResponse
	if err := json.Unmarshal(body, &tagBindings); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Retrieved %d tag bindings for workspace %s", len(tagBindings.Data), workspaceID)
	return &tagBindings, nil
}

// CreateWorkspaceTagBindings creates tag bindings for a workspace
func (c *Client) CreateWorkspaceTagBindings(token, workspaceID string, request *TagBindingCreateRequest) (*TagBindingCreateResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("tag binding create request is required")
	}

	// Validate required fields
	for i, data := range request.Data {
		if data.Type != "tag-bindings" {
			return nil, NewValidationError(fmt.Sprintf("request data[%d] type must be 'tag-bindings'", i))
		}
		if data.Attributes.Key == "" {
			return nil, NewValidationError(fmt.Sprintf("request data[%d] key is required", i))
		}
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/tag-bindings", c.baseURL, workspaceID)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Creating workspace tag bindings at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Create tag bindings response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var tagBindings TagBindingCreateResponse
	if err := json.Unmarshal(body, &tagBindings); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Created %d tag bindings for workspace %s", len(tagBindings.Data), workspaceID)
	return &tagBindings, nil
}

// UpdateWorkspaceTagBindings updates existing tag bindings for a workspace
func (c *Client) UpdateWorkspaceTagBindings(token, workspaceID string, request *TagBindingUpdateRequest) (*TagBindingCreateResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}
	if request == nil {
		return nil, NewValidationError("tag binding update request is required")
	}

	// Validate required fields
	for i, data := range request.Data {
		if data.Type != "tag-bindings" {
			return nil, NewValidationError(fmt.Sprintf("request data[%d] type must be 'tag-bindings'", i))
		}
		if data.ID == "" {
			return nil, NewValidationError(fmt.Sprintf("request data[%d] ID is required", i))
		}
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/tag-bindings", c.baseURL, workspaceID)

	// Marshal request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Updating workspace tag bindings at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("PATCH", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Update tag bindings response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var tagBindings TagBindingCreateResponse
	if err := json.Unmarshal(body, &tagBindings); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Updated %d tag bindings for workspace %s", len(tagBindings.Data), workspaceID)
	return &tagBindings, nil
}

// DeleteWorkspaceTagBindings deletes tag bindings from a workspace
func (c *Client) DeleteWorkspaceTagBindings(token, workspaceID string, tagBindingIDs []string) error {
	if token == "" {
		return NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return NewValidationError("workspace ID is required")
	}
	if len(tagBindingIDs) == 0 {
		return NewValidationError("at least one tag binding ID is required")
	}

	// Delete each tag binding individually (HCP Terraform API doesn't support bulk delete)
	for _, tagBindingID := range tagBindingIDs {
		// Build URL
		endpoint := fmt.Sprintf("%s/workspaces/%s/tag-bindings/%s", c.baseURL, workspaceID, tagBindingID)

		c.logger.Debugf("Deleting tag binding at: %s", endpoint)

		// Create request
		req, err := http.NewRequest("DELETE", endpoint, nil)
		if err != nil {
			return err
		}

		// Set headers
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("Content-Type", "application/vnd.api+json")

		// Make request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return NewErrorFromResponse(resp, nil)
		}

		c.logger.Infof("Deleted tag binding %s from workspace %s", tagBindingID, workspaceID)
	}

	c.logger.Infof("Deleted %d tag bindings from workspace %s", len(tagBindingIDs), workspaceID)
	return nil
}

// LockWorkspace locks a workspace to prevent concurrent operations
func (c *Client) LockWorkspace(token, workspaceID string, reason *string) (*WorkspaceLockResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build request body
	request := WorkspaceLockRequest{}
	if reason != nil {
		request.Reason = *reason
	} else {
		request.Reason = "Locked via Terraform MCP Server"
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/actions/lock", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Locking workspace %s at: %s", workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Lock workspace response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var lockResponse WorkspaceLockResponse
	if err := json.Unmarshal(body, &lockResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Locked workspace %s", workspaceID)
	return &lockResponse, nil
}

// UnlockWorkspace unlocks a workspace to allow operations
func (c *Client) UnlockWorkspace(token, workspaceID string, forceUnlock *bool) (*WorkspaceLockResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build request body
	request := WorkspaceUnlockRequest{}
	if forceUnlock != nil {
		request.ForceUnlock = forceUnlock
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/actions/unlock", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Unlocking workspace %s at: %s", workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Unlock workspace response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var lockResponse WorkspaceLockResponse
	if err := json.Unmarshal(body, &lockResponse); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Unlocked workspace %s", workspaceID)
	return &lockResponse, nil
}

// GetWorkspaceRemoteStateConsumers retrieves workspaces that can access this workspace's state
func (c *Client) GetWorkspaceRemoteStateConsumers(token, workspaceID string) (*RemoteStateConsumersResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return nil, NewValidationError("workspace ID is required")
	}

	// Build URL - correct endpoint for remote state consumers
	endpoint := fmt.Sprintf("%s/workspaces/%s/relationships/remote-state-consumers", c.baseURL, workspaceID)

	c.logger.Debugf("Getting remote state consumers for workspace %s at: %s", workspaceID, endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.logger.Tracef("Get remote state consumers response: %s", string(body))

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse JSON response
	var consumers RemoteStateConsumersResponse
	if err := json.Unmarshal(body, &consumers); err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to parse response JSON: %v", err))
	}

	c.logger.Infof("Retrieved %d remote state consumers for workspace %s", len(consumers.Data), workspaceID)
	return &consumers, nil
}

// AddWorkspaceRemoteStateConsumers adds workspaces as remote state consumers
func (c *Client) AddWorkspaceRemoteStateConsumers(token, workspaceID string, consumerWorkspaceIDs []string) error {
	if token == "" {
		return NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return NewValidationError("workspace ID is required")
	}
	if len(consumerWorkspaceIDs) == 0 {
		return NewValidationError("at least one consumer workspace ID is required")
	}

	// Build request data
	var requestData []RemoteStateConsumerData
	for _, consumerID := range consumerWorkspaceIDs {
		requestData = append(requestData, RemoteStateConsumerData{
			Type: "workspaces",
			ID:   consumerID,
		})
	}

	request := RemoteStateConsumerRequest{
		Data: requestData,
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/relationships/remote-state-consumers", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Adding remote state consumers to workspace %s at: %s", workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewErrorFromResponse(resp, nil)
	}

	c.logger.Infof("Added %d remote state consumers to workspace %s", len(consumerWorkspaceIDs), workspaceID)
	return nil
}

// RemoveWorkspaceRemoteStateConsumers removes workspaces as remote state consumers
func (c *Client) RemoveWorkspaceRemoteStateConsumers(token, workspaceID string, consumerWorkspaceIDs []string) error {
	if token == "" {
		return NewAuthenticationError("authentication token is required")
	}
	if workspaceID == "" {
		return NewValidationError("workspace ID is required")
	}
	if len(consumerWorkspaceIDs) == 0 {
		return NewValidationError("at least one consumer workspace ID is required")
	}

	// Build request data
	var requestData []RemoteStateConsumerData
	for _, consumerID := range consumerWorkspaceIDs {
		requestData = append(requestData, RemoteStateConsumerData{
			Type: "workspaces",
			ID:   consumerID,
		})
	}

	request := RemoteStateConsumerRequest{
		Data: requestData,
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/workspaces/%s/relationships/remote-state-consumers", c.baseURL, workspaceID)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Removing remote state consumers from workspace %s at: %s", workspaceID, endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("DELETE", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewErrorFromResponse(resp, nil)
	}

	c.logger.Infof("Removed %d remote state consumers from workspace %s", len(consumerWorkspaceIDs), workspaceID)
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

// ====================
// Run Methods
// ====================

// CreateRun creates a new run for a workspace
func (c *Client) CreateRun(token string, request *RunCreateRequest) (*SingleRunResponse, error) {
	endpoint := fmt.Sprintf("%s/runs", c.baseURL)

	// Convert request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request: %v", err))
	}

	c.logger.Debugf("Creating run at: %s", endpoint)
	c.logger.Tracef("Request body: %s", string(requestBody))

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Infof("Created run %s with status: %s", response.Data.ID, response.Data.Attributes.Status)
	return &response, nil
}

// GetRun retrieves a specific run by ID
func (c *Client) GetRun(token, runID string, include []string) (*SingleRunResponse, error) {
	endpoint := fmt.Sprintf("%s/runs/%s", c.baseURL, runID)

	// Add include parameter if provided
	if len(include) > 0 {
		params := url.Values{}
		params.Set("include", strings.Join(include, ","))
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
	}

	c.logger.Debugf("Getting run at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Retrieved run %s with status: %s", response.Data.ID, response.Data.Attributes.Status)
	return &response, nil
}

// ListRuns lists runs for a workspace with optional filtering
func (c *Client) ListRuns(token, workspaceID string, options *RunListOptions) (*RunResponse, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%s/runs", c.baseURL, workspaceID)

	// Build query parameters
	params := url.Values{}
	if options != nil {
		if options.PageSize > 0 {
			params.Set("page[size]", strconv.Itoa(options.PageSize))
		}
		if options.PageNumber > 0 {
			params.Set("page[number]", strconv.Itoa(options.PageNumber))
		}
		if options.Status != "" {
			params.Set("filter[status]", options.Status)
		}
		if options.Operation != "" {
			params.Set("filter[operation]", options.Operation)
		}
		if options.Source != "" {
			params.Set("filter[source]", options.Source)
		}
		if len(options.Include) > 0 {
			params.Set("include", strings.Join(options.Include, ","))
		}
	}

	if len(params) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, params.Encode())
	}

	c.logger.Debugf("Listing runs at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response RunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Retrieved %d runs for workspace %s", len(response.Data), workspaceID)
	return &response, nil
}

// ApplyRun applies a planned run
func (c *Client) ApplyRun(token, runID, comment string) (*SingleRunResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if runID == "" {
		return nil, NewValidationError("run ID is required")
	}

	endpoint := fmt.Sprintf("%s/runs/%s/actions/apply", c.baseURL, runID)

	// Build request body
	applyRequest := map[string]interface{}{
		"comment": comment,
	}

	requestBody, err := json.Marshal(applyRequest)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Applying run at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Successfully applied run %s", runID)
	return &response, nil
}

// DiscardRun discards a planned run
func (c *Client) DiscardRun(token, runID, comment string) (*SingleRunResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if runID == "" {
		return nil, NewValidationError("run ID is required")
	}

	endpoint := fmt.Sprintf("%s/runs/%s/actions/discard", c.baseURL, runID)

	// Build request body
	discardRequest := map[string]interface{}{
		"comment": comment,
	}

	requestBody, err := json.Marshal(discardRequest)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Discarding run at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Successfully discarded run %s", runID)
	return &response, nil
}

// CancelRun cancels a running run
func (c *Client) CancelRun(token, runID, comment string, forceCancel bool) (*SingleRunResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if runID == "" {
		return nil, NewValidationError("run ID is required")
	}

	var endpoint string
	if forceCancel {
		endpoint = fmt.Sprintf("%s/runs/%s/actions/force-cancel", c.baseURL, runID)
	} else {
		endpoint = fmt.Sprintf("%s/runs/%s/actions/cancel", c.baseURL, runID)
	}

	// Build request body
	cancelRequest := map[string]interface{}{
		"comment": comment,
	}

	requestBody, err := json.Marshal(cancelRequest)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("failed to marshal request body: %v", err))
	}

	c.logger.Debugf("Canceling run at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleRunResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	action := "canceled"
	if forceCancel {
		action = "force-canceled"
	}
	c.logger.Debugf("Successfully %s run %s", action, runID)
	return &response, nil
}

// GetPlan gets detailed information about a plan
func (c *Client) GetPlan(token, planID string) (*SinglePlanResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if planID == "" {
		return nil, NewValidationError("plan ID is required")
	}

	endpoint := fmt.Sprintf("%s/plans/%s", c.baseURL, planID)

	c.logger.Debugf("Getting plan at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SinglePlanResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Retrieved plan %s", planID)
	return &response, nil
}

// GetApply gets detailed information about an apply
func (c *Client) GetApply(token, applyID string) (*SingleApplyResponse, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if applyID == "" {
		return nil, NewValidationError("apply ID is required")
	}

	endpoint := fmt.Sprintf("%s/applies/%s", c.baseURL, applyID)

	c.logger.Debugf("Getting apply at: %s", endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/vnd.api+json")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewErrorFromResponse(resp, nil)
	}

	// Parse response
	var response SingleApplyResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debugf("Retrieved apply %s", applyID)
	return &response, nil
}

// GetRunLogs gets logs for a specific run
func (c *Client) GetRunLogs(token, runID, logType string) (string, error) {
	if token == "" {
		return "", NewAuthenticationError("authentication token is required")
	}
	if runID == "" {
		return "", NewValidationError("run ID is required")
	}
	if logType == "" {
		return "", NewValidationError("log type is required")
	}

	var endpoint string
	if logType == "plan" {
		// Get plan ID from run first, then fetch plan logs
		run, err := c.GetRun(token, runID, []string{"plan"})
		if err != nil {
			return "", fmt.Errorf("failed to get run details: %w", err)
		}

		// Extract plan ID from run relationships
		planID, err := extractRelationshipID(run.Data.Relationships.Plan)
		if err != nil {
			return "", fmt.Errorf("no plan found for run %s: %w", runID, err)
		}

		endpoint = fmt.Sprintf("%s/plans/%s/log", c.baseURL, planID)
	} else if logType == "apply" {
		// Get apply ID from run first, then fetch apply logs
		run, err := c.GetRun(token, runID, []string{"apply"})
		if err != nil {
			return "", fmt.Errorf("failed to get run details: %w", err)
		}

		// Extract apply ID from run relationships
		applyID, err := extractRelationshipID(run.Data.Relationships.Apply)
		if err != nil {
			return "", fmt.Errorf("no apply found for run %s: %w", runID, err)
		}

		endpoint = fmt.Sprintf("%s/applies/%s/log", c.baseURL, applyID)
	} else {
		return "", NewValidationError("log_type must be 'plan' or 'apply'")
	}

	c.logger.Debugf("Getting %s logs at: %s", logType, endpoint)

	// Create request
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", NewErrorFromResponse(resp, nil)
	}

	c.logger.Debugf("Retrieved %s logs for run %s", logType, runID)
	return string(body), nil
}

// GetRunOutput gets structured output for a run including plan and apply details
func (c *Client) GetRunOutput(token, runID string, includePlan, includeApply bool) (map[string]interface{}, error) {
	if token == "" {
		return nil, NewAuthenticationError("authentication token is required")
	}
	if runID == "" {
		return nil, NewValidationError("run ID is required")
	}

	c.logger.Debugf("Getting run output for: %s", runID)

	// Get run details with plan and apply relationships
	include := []string{}
	if includePlan {
		include = append(include, "plan")
	}
	if includeApply {
		include = append(include, "apply")
	}

	run, err := c.GetRun(token, runID, include)
	if err != nil {
		return nil, fmt.Errorf("failed to get run details: %w", err)
	}

	result := map[string]interface{}{
		"run_id": runID,
		"run":    run.Data,
	}

	// Get plan details if requested and available
	if includePlan {
		planID, err := extractRelationshipID(run.Data.Relationships.Plan)
		if err == nil {
			plan, err := c.GetPlan(token, planID)
			if err != nil {
				c.logger.Warnf("Failed to get plan details: %v", err)
			} else {
				result["plan"] = plan.Data
			}
		}
	}

	// Get apply details if requested and available
	if includeApply {
		applyID, err := extractRelationshipID(run.Data.Relationships.Apply)
		if err == nil {
			apply, err := c.GetApply(token, applyID)
			if err != nil {
				c.logger.Warnf("Failed to get apply details: %v", err)
			} else {
				result["apply"] = apply.Data
			}
		}
	}

	c.logger.Debugf("Retrieved run output for %s", runID)
	return result, nil
}

// extractRelationshipID extracts the ID from a RelationshipData object
func extractRelationshipID(rel *RelationshipData) (string, error) {
	if rel == nil {
		return "", fmt.Errorf("relationship is nil")
	}

	// Handle the case where Data is a map
	if dataMap, ok := rel.Data.(map[string]interface{}); ok {
		if id, exists := dataMap["id"]; exists {
			if idStr, ok := id.(string); ok {
				return idStr, nil
			}
		}
	}

	// Handle the case where Data is a struct with ID field
	if dataStruct, ok := rel.Data.(struct{ ID string }); ok {
		return dataStruct.ID, nil
	}

	return "", fmt.Errorf("could not extract ID from relationship data")
}
