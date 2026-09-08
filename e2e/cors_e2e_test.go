// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Raw JSON types used to initialize HTTP MCP sessions while preserving Origin.
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

// TestCORSE2E checks allowed origins, rejected origins, and preflight requests.
func TestCORSE2E(t *testing.T) {
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
		config := config

		t.Run(config.name, func(t *testing.T) {
			// CORS checks use the legacy endpoint because the raw initializer is
			// currently compatible with that endpoint only.
			serverConfig := legacyServerConfig()
			baseURL := fmt.Sprintf("http://localhost:%s", config.port)
			mcpURL := baseURL + serverConfig.mcpPath

			containerID := startHTTPContainerWithCORS(
				t,
				config.port,
				config.mode,
				config.origins,
				officialSDKEnabled(),
			)

			// Each mode uses its own container and host port so its CORS configuration
			// is isolated from the other modes.
			t.Cleanup(func() {
				stopContainer(t, containerID)
			})

			waitForServer(t, baseURL)
			runCORSTests(t, mcpURL, config.mode, serverConfig.serverName)
		})
	}
}

// runCORSTests executes the shared and mode-specific CORS cases.
func runCORSTests(t *testing.T, mcpURL, mode, expectedServerName string) {
	// Describe one HTTP request and its expected CORS response.
	type testCase struct {
		name              string
		method            string
		origin            string
		expectedStatus    int
		expectCORSHeaders bool
	}

	// All CORS modes share these basic allowed, missing-origin, and preflight checks.
	baseTestCases := []testCase{
		{"GET with allowed origin", "GET", "https://example.com", 200, true},
		{"GET with no origin", "GET", "", 200, false},
		{"OPTIONS preflight with allowed origin", "OPTIONS", "https://example.com", 200, true},
	}

	// Add checks that differ by the selected CORS policy.
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

	// Add expectations specific to the selected CORS mode.
	switch mode {
	case "strict":
		testCases = append(testCases, strictModeTests...)
	case "development":
		testCases = append(testCases, developmentModeTests...)
	case "disabled":
		testCases = append(testCases, disabledModeTests...)
	}

	// Run each request as a named subtest for clear failure output.
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Non-OPTIONS requests need a session before the MCP endpoint can be called.
			var sessionID string
			if tc.method != "OPTIONS" {
				// Initialize only for origins expected to pass the CORS policy.
				if tc.expectedStatus == 200 {
					sessionID = initializeMCPSession(t, mcpURL, tc.origin, expectedServerName)
					require.NotEmpty(t, sessionID, "Expected to get a session ID for allowed origin")
				} else {
					// Rejected origins should fail at the CORS middleware before MCP initialization.
					testCORSDirectly(t, mcpURL, tc.method, tc.origin, tc.expectedStatus, tc.expectCORSHeaders)
					return
				}
			}

			// Send the request with the Origin and session headers under test.
			client := &http.Client{}
			var body []byte
			if tc.method != "OPTIONS" && sessionID != "" {
				// Use a tool-call-shaped body; the tool result is irrelevant to this CORS test.
				callToolReq := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name":      "ping", // Only the CORS response is being checked.
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

// testCORSDirectly checks rejected requests without creating an MCP session.
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

// initializeMCPSession performs raw initialization so the Origin header is controlled.
func initializeMCPSession(t *testing.T, mcpURL, origin, expectedServerName string) string {
	// Build the smallest valid initialize request needed to obtain a session ID.
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

	// Send initialization with the origin being tested.
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check if we got a successful response
	require.Equal(t, 200, resp.StatusCode, "Failed to initialize MCP session")

	// The session ID is required for the following streamable HTTP request.
	sessionID := resp.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID, "Expected to receive a session ID")

	// Verify we got a valid response
	var initResp InitializeResponse
	err = json.NewDecoder(resp.Body).Decode(&initResp)
	require.NoError(t, err)
	assert.Equal(t, expectedServerName, initResp.Result.ServerInfo.Name)

	return sessionID
}
