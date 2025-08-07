// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MCP initialization payload structure
type InitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type InitializeRequest struct {
	Jsonrpc string           `json:"jsonrpc"`
	Method  string           `json:"method"`
	Params  InitializeParams `json:"params"`
	ID      int              `json:"id"`
}

type InitializeResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	} `json:"result"`
	ID int `json:"id"`
}

// TestCORSE2E tests CORS validation in the MCP server using direct HTTP requests
func TestCORSE2E(t *testing.T) {
	// Build the Docker image for our tests
	buildDockerImage(t)

	// Ensure all test containers are cleaned up at the end
	t.Cleanup(func() {
		cleanupAllTestContainers(t)
	})

	// Define test configurations for different CORS modes
	corsConfigs := []struct {
		name    string
		mode    string
		origins string
		port    string
	}{
		{"strict mode", "strict", "https://example.com,https://allowed.com", "8081"},
		{"development mode", "development", "https://example.com", "8082"},
		{"disabled mode", "disabled", "", "8083"},
	}

	for _, config := range corsConfigs {
		t.Run(config.name, func(t *testing.T) {
			// Start server with specific CORS config
			baseURL := fmt.Sprintf("http://localhost:%s", config.port)
			mcpURL := fmt.Sprintf("%s/mcp", baseURL)

			containerID := startHTTPContainerWithCORS(t, config.port, config.mode, config.origins)
			defer func() {
				stopCmd := exec.Command("docker", "stop", containerID)
				stopCmd.Run()
			}()

			waitForCORSServer(t, baseURL)

			// Now run the specific CORS tests for this configuration
			runCORSTests(t, mcpURL, config.mode, config.origins)
		})
	}
}

// startHTTPContainerWithCORS starts a Docker container with specific CORS settings
func startHTTPContainerWithCORS(t *testing.T, port, mode, origins string) string {
	portMapping := fmt.Sprintf("%s:8080", port)
	cmd := exec.Command(
		"docker", "run", "-d", "--rm",
		"-e", "TRANSPORT_MODE=streamable-http",
		"-e", "TRANSPORT_HOST=0.0.0.0",
		"-e", "MCP_SESSION_MODE=stateful",
		"-e", fmt.Sprintf("MCP_CORS_MODE=%s", mode),
		"-e", fmt.Sprintf("MCP_ALLOWED_ORIGINS=%s", origins),
		"-p", portMapping,
		"terraform-mcp-server:test-e2e",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Docker command failed: %s", string(output))
		require.NoError(t, err, "expected to start HTTP container successfully")
	}

	containerID := strings.TrimSpace(string(output))[:12] // First 12 chars of container ID
	t.Logf("Started HTTP container: %s on port %s with CORS mode: %s, origins: %s",
		containerID, port, mode, origins)
	return containerID
}

// waitForCORSServer waits for the HTTP server to be ready
func waitForCORSServer(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 2 * time.Second}
	for range 30 {
		resp, err := client.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			t.Log("HTTP server is ready")
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatal("HTTP server failed to start within 30 seconds")
}

// runCORSTests executes the CORS test cases for a specific configuration
func runCORSTests(t *testing.T, mcpURL, mode, configuredOrigins string) {
	// Parse the configured origins
	allowedOrigins := []string{}
	if configuredOrigins != "" {
		for _, origin := range strings.Split(configuredOrigins, ",") {
			allowedOrigins = append(allowedOrigins, strings.TrimSpace(origin))
		}
	}

	// Define the test case struct type
	type testCase struct {
		name              string
		method            string
		origin            string
		expectedStatus    int
		expectCORSHeaders bool
	}

	// Define base test cases that apply to all modes
	baseTestCases := []testCase{
		{"GET with allowed origin", "GET", "https://example.com", 200, true},
		{"GET with no origin", "GET", "", 200, false},
		{"OPTIONS preflight with allowed origin", "OPTIONS", "https://example.com", 200, true},
	}

	// Define mode-specific test cases
	strictModeTests := []testCase{
		{"GET with disallowed origin", "GET", "https://evil.com", 403, false},
		{"GET with localhost origin", "GET", "http://localhost:3000", 403, false},
		{"OPTIONS with disallowed origin", "OPTIONS", "https://evil.com", 403, false},
	}

	developmentModeTests := []testCase{
		{"GET with localhost origin", "GET", "http://localhost:3000", 200, true},
		{"GET with IPv4 localhost", "GET", "http://127.0.0.1:3000", 200, true},
		{"GET with IPv6 localhost", "GET", "http://[::1]:3000", 200, true},
		{"GET with disallowed origin", "GET", "https://evil.com", 403, false},
		{"OPTIONS with localhost origin", "OPTIONS", "http://localhost:3000", 200, true},
	}

	disabledModeTests := []testCase{
		{"GET with any origin", "GET", "https://any-site.com", 200, true},
		{"OPTIONS with any origin", "OPTIONS", "https://any-site.com", 200, true},
	}

	// Start with base test cases
	testCases := baseTestCases

	// Add mode-specific test cases
	switch mode {
	case "strict":
		testCases = append(testCases, strictModeTests...)
	case "development":
		testCases = append(testCases, developmentModeTests...)
	case "disabled":
		testCases = append(testCases, disabledModeTests...)
	}

	// Run the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For non-OPTIONS requests, we need to initialize the MCP session first
			var sessionID string
			if tc.method != "OPTIONS" {
				// Only try to initialize if we expect it to succeed
				if tc.expectedStatus == 200 {
					sessionID = initializeMCPSession(t, mcpURL, tc.origin)
					require.NotEmpty(t, sessionID, "Expected to get a session ID for allowed origin")
				} else {
					// For requests we expect to fail, just check the CORS directly
					testCORSDirectly(t, mcpURL, tc.method, tc.origin, tc.expectedStatus, tc.expectCORSHeaders)
					return
				}
			}

			// Now make the test request
			client := &http.Client{}
			var body []byte
			if tc.method != "OPTIONS" && sessionID != "" {
				// For non-OPTIONS requests with a session, we need a valid MCP request
				callToolReq := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "callTool",
					"params": map[string]interface{}{
						"name":      "ping", // A dummy tool name just to have something
						"arguments": map[string]interface{}{},
					},
					"id": 2,
				}
				body, _ = json.Marshal(callToolReq)
			}

			req, _ := http.NewRequest(tc.method, mcpURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			// Add the session ID if we have one
			if sessionID != "" {
				req.Header.Set("Mcp-Session-Id", sessionID)
			}

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode, "Unexpected status code")

			if tc.expectCORSHeaders {
				assert.Equal(t, tc.origin, resp.Header.Get("Access-Control-Allow-Origin"),
					"Expected Access-Control-Allow-Origin header to match origin")
				assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Methods"),
					"Expected Access-Control-Allow-Methods header to be set")
			} else if resp.StatusCode == 200 || resp.StatusCode == 202 {
				// If status is 200 but we don't expect CORS headers (e.g., no origin case)
				if tc.origin == "" {
					assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"),
						"Expected no Access-Control-Allow-Origin header when no origin is sent")
				}
			}
		})
	}
}

// testCORSDirectly tests CORS behavior directly without trying to establish a session
func testCORSDirectly(t *testing.T, mcpURL, method, origin string, expectedStatus int, expectCORSHeaders bool) {
	client := &http.Client{}
	req, _ := http.NewRequest(method, mcpURL, nil)

	if origin != "" {
		req.Header.Set("Origin", origin)
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, expectedStatus, resp.StatusCode, "Unexpected status code")

	if expectCORSHeaders {
		assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"),
			"Expected Access-Control-Allow-Origin header to match origin")
		assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Methods"),
			"Expected Access-Control-Allow-Methods header to be set")
	} else if resp.StatusCode == 200 || resp.StatusCode == 202 {
		if origin == "" {
			assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"),
				"Expected no Access-Control-Allow-Origin header when no origin is sent")
		}
	}
}

// initializeMCPSession initializes an MCP session and returns the session ID
func initializeMCPSession(t *testing.T, mcpURL, origin string) string {
	// Create the initialization payload
	initReq := InitializeRequest{
		Jsonrpc: "2.0",
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: "0.1.0", // Use the latest version
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "cors-e2e-test-client",
				Version: "0.0.1",
			},
		},
		ID: 1,
	}

	// Convert to JSON
	payload, err := json.Marshal(initReq)
	require.NoError(t, err)

	// Create the request
	client := &http.Client{}
	req, err := http.NewRequest("POST", mcpURL, bytes.NewBuffer(payload))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}

	// Send the request
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check if we got a successful response
	require.Equal(t, 200, resp.StatusCode, "Failed to initialize MCP session")

	// Extract the session ID from the response headers
	sessionID := resp.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID, "Expected to receive a session ID")

	// Verify we got a valid response
	var initResp InitializeResponse
	err = json.NewDecoder(resp.Body).Decode(&initResp)
	require.NoError(t, err)
	assert.Equal(t, "terraform-mcp-server", initResp.Result.ServerInfo.Name)

	return sessionID
}
