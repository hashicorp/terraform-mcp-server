// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestE2E(t *testing.T) {
	buildDockerImage(t)

	// Ensure all test containers are cleaned up at the end
	t.Cleanup(func() {
		cleanupAllTestContainers(t)
	})

	testCases := []struct {
		name          string
		clientFactory func(t *testing.T) (*mcp.ClientSession, func())
	}{
		{"Stdio", createStdioClient},
		{"HTTP", createHTTPClient},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session, cleanup := tc.clientFactory(t)
			defer cleanup()
			runTestSuite(t, session, tc.name)
		})
	}
}

// ensureClientInitialized ensures the MCP client is initialized before running tool tests
func ensureClientInitialized(t *testing.T, client *mcp.ClientSession) {
	result := client.InitializeResult()
	require.NotNil(t, result)
	t.Logf("Initialized with server: %s %s", result.ServerInfo.Name, result.ServerInfo.Version)
	require.Equal(t, "terraform-mcp-official", result.ServerInfo.Name)
}

// runTestSuite executes all test cases against the provided client
func runTestSuite(t *testing.T, client *mcp.ClientSession, transportName string) {
	t.Run("Initialize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{
			Name:    "e2e-test-client",
			Version: "0.0.1",
		}, nil)

		var transport mcp.Transport
		_, thisFile, _, ok := runtime.Caller(0)
		require.True(t, ok)
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
		serverBinaryPath := filepath.Join(t.TempDir(), "terraform-mcp-server-official")
		cmd := exec.Command("go", "build", "-o", serverBinaryPath, "./cmd/terraform-mcp-server-official")
		cmd.Dir = repoRoot
		require.NoError(t, cmd.Run())
		switch transportName {
		case "Stdio":
			transport = &mcp.CommandTransport{
				Command: exec.Command(serverBinaryPath),
			}
		case "HTTP":

		default:
			t.Fatalf("unsupported transport: %s", transportName)
		}

		session, err := client.Connect(ctx, transport, nil)
		require.NoError(t, err)

		defer session.Close()

		result := session.InitializeResult()
		require.NotNil(t, result)

		fmt.Printf(
			"Initialized with server: %s %s\n\n",
			result.ServerInfo.Name,
			result.ServerInfo.Version,
		)
		require.Equal(t, "terraform-mcp-official", result.ServerInfo.Name)
	})

	for _, testCase := range searchProviderTestCases {
		t.Run(fmt.Sprintf("%s_search_providers/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL search_providers %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "search_providers"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'search_providers' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'search_providers' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Content length: %d", len(textContent.Text))

				switch testCase.TestContentType {
				case CONST_TYPE_DATA_SOURCE:
					require.Contains(t, textContent.Text, "Category: data-sources", "expected content to contain data-sources")
				case CONST_TYPE_RESOURCE:
					require.Contains(t, textContent.Text, "Category: resources", "expected content to contain resources")
				case CONST_TYPE_GUIDES:
					require.Contains(t, textContent.Text, "guide", "expected content to contain guide")
				case CONST_TYPE_FUNCTIONS:
					require.Contains(t, textContent.Text, "functions", "expected content to contain functions")
				case CONST_TYPE_ACTIONS:
					require.Contains(t, textContent.Text, "actions", "expected content to contain actions")
				case CONST_TYPE_LIST_RESOURCES:
					require.Contains(t, textContent.Text, "list-resources", "expected content to contain list-resources")
				}
			}
		})
	}

	for _, testCase := range providerDetailsTestCases {
		t.Run(fmt.Sprintf("%s_get_provider_details/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL get_provider_details %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "get_provider_details"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'get_provider_details' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'get_provider_details' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Content length: %d", len(textContent.Text))

				require.Contains(t, textContent.Text, "page_title", "expected content to contain a page_title")
			}
		})
	}

	for _, testCase := range searchModulesTestCases {
		t.Run(fmt.Sprintf("%s_search_modules/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL search_modules %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "search_modules"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'search_modules' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'search_modules' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				if len(response.Content) > 0 {
					textContent, ok := response.Content[0].(*mcp.TextContent)
					require.True(t, ok, "expected content to be of type TextContent")
					t.Logf("Content length: %d", len(textContent.Text))
				} else {
					t.Log("Response content is empty for successful call.")
				}
			}
		})
	}

	for _, testCase := range moduleDetailsTestCases {
		t.Run(fmt.Sprintf("%s_get_module_details/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL get_module_details %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "get_module_details"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'get_module_details' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'get_module_details' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Content length: %d", len(textContent.Text))

				switch testCase.TestContentType {
				case CONST_TYPE_DATA_SOURCE:
					require.NotContains(t, textContent.Text, "**Category:** resources", "expected content not to contain resources")
				case CONST_TYPE_RESOURCE:
					require.NotContains(t, textContent.Text, "**Category:** data-sources", "expected content not to contain data-sources")
				}
			}
		})
	}

	for _, testCase := range searchPoliciesTestCases {
		t.Run("CallTool search_policies", func(t *testing.T) {
			// t.Parallel()
			t.Logf("TOOL search_policies %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "search_policies"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'search_policies' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'search_policies' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Content length: %d", len(textContent.Text))

				// For successful searches, check that the response contains the expected policy information format
				if len(textContent.Text) > 0 {
					require.Contains(t, textContent.Text, "terraform_policy_id", "expected content to contain terraform_policy_id")
					require.Contains(t, textContent.Text, "Name:", "expected content to contain policy Name")
					require.Contains(t, textContent.Text, "Title:", "expected content to contain policy Title")
					require.Contains(t, textContent.Text, "Downloads:", "expected content to contain Downloads count")
				}
			}
		})
	}

	for _, testCase := range policyDetailsTestCases {
		t.Run("CallTool get_policy_details", func(t *testing.T) {
			// t.Parallel()
			t.Logf("TOOL get_policy_details %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "get_policy_details"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'get_policy_details' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'get_policy_details' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have at least one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Content length: %d", len(textContent.Text))

				// Add specific assertions for policy details if needed
				require.Contains(t, textContent.Text, "POLICY_NAME", "expected content to contain policy name")
				require.Contains(t, textContent.Text, "POLICY_CHECKSUM:", "expected content to contain policy checksum")
			}
		})
	}

	for _, testCase := range getLatestModuleVersionTestCases {
		t.Run(fmt.Sprintf("%s_get_latest_module_version/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL get_latest_module_version %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "get_latest_module_version"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'get_latest_module_version' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'get_latest_module_version' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Module version: %s", textContent.Text)

				// Verify that the response contains a valid version string
				require.NotEmpty(t, textContent.Text, "expected version string to not be empty")
				// Basic version format validation (should contain at least one dot for semantic versioning)
				require.Contains(t, textContent.Text, ".", "expected version to contain at least one dot")
			}
		})
	}

	for _, testCase := range getLatestProviderVersionTestCases {
		t.Run(fmt.Sprintf("%s_get_latest_provider_version/%s", transportName, testCase.TestName), func(t *testing.T) {
			ensureClientInitialized(t, client)
			t.Logf("TOOL get_latest_provider_version %s", testCase.TestDescription)
			t.Logf("Test payload: %v", testCase.TestPayload)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			request := &mcp.CallToolParams{}
			request.Name = "get_latest_provider_version"
			request.Arguments = testCase.TestPayload

			response, err := client.CallTool(ctx, request)
			if testCase.TestShouldFail {
				require.NoError(t, err)
				require.True(t, response.IsError, "expected to call 'get_latest_provider_version' tool with error")
			} else {
				require.NoError(t, err, "expected to call 'get_latest_provider_version' tool successfully")
				require.False(t, response.IsError, "expected result not to be an error")
				require.Len(t, response.Content, 1, "expected content to have one item")

				textContent, ok := response.Content[0].(*mcp.TextContent)
				require.True(t, ok, "expected content to be of type TextContent")
				t.Logf("Provider version: %s", textContent.Text)

				// Verify that the response contains a valid version string
				require.NotEmpty(t, textContent.Text, "expected version string to not be empty")
				// Basic version format validation (should contain at least one dot for semantic versioning)
				require.Contains(t, textContent.Text, ".", "expected version to contain at least one dot")
			}
		})
	}

}

// createStdioClient creates a stdio-based MCP client
func createStdioClient(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "test",
	}, nil)

	transport := &mcp.CommandTransport{
		Command: exec.Command(
			"docker",
			"run",
			"-i",
			"--rm",
			"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
			"-e", "MCP_RATE_LIMIT_SESSION=50:100",
			"terraform-mcp-server-official:test-e2e-official"),
	}
	t.Log("Starting Stdio MCP client...")
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}

	return session, func() { _ = session.Close() }
}

// createHTTPClient creates an HTTP-based MCP client
func createHTTPClient(t *testing.T) (*mcp.ClientSession, func()) {
	t.Log("Starting HTTP MCP server...")

	port := getTestPort()
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "test",
	}, nil)

	transport := &mcp.StreamableClientTransport{
		Endpoint: baseURL,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}

	return session, func() { _ = session.Close() }
}

// startHTTPContainer starts a Docker container in HTTP mode and returns container ID
func startHTTPContainer(t *testing.T, port string) string {
	portMapping := fmt.Sprintf("%s:8080", port)
	cmd := exec.Command(
		"docker", "run", "-d", "--rm",
		"-e", "TRANSPORT_MODE=streamable-http",
		"-e", "TRANSPORT_HOST=0.0.0.0",
		"-e", "MCP_SESSION_MODE=stateful",
		"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
		"-e", "MCP_RATE_LIMIT_SESSION=50:100",
		"-p", portMapping,
		"terraform-mcp-server-official:test-e2e-official",
	)
	output, err := cmd.Output()
	require.NoError(t, err, "expected to start HTTP container successfully")

	containerID := string(output)[:12] // First 12 chars of container ID
	t.Logf("Started HTTP container: %s on port %s", containerID, port)
	return containerID
}

// waitForServer waits for the HTTP server to be ready
func waitForServer(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 30; i++ {
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

// stopContainer stops the Docker container
func stopContainer(t *testing.T, containerID string) {
	if containerID == "" {
		return
	}

	t.Logf("Stopping container: %s", containerID)
	cmd := exec.Command("docker", "stop", containerID)
	if err := cmd.Run(); err != nil {
		t.Logf("Warning: failed to stop container %s: %v", containerID, err)
		// Try force kill if stop fails
		killCmd := exec.Command("docker", "kill", containerID)
		if killErr := killCmd.Run(); killErr != nil {
			t.Logf("Warning: failed to kill container %s: %v", containerID, killErr)
		}
	} else {
		t.Logf("Successfully stopped container: %s", containerID)
	}
}

// cleanupAllTestContainers stops all containers created by this test
func cleanupAllTestContainers(t *testing.T) {
	t.Log("Cleaning up all test containers...")

	// Find all containers with our test image
	cmd := exec.Command("docker", "ps", "-q", "--filter", "ancestor=terraform-mcp-server-official:test-e2e-official")
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Warning: failed to list test containers: %v", err)
		return
	}

	containerIDs := string(output)
	if containerIDs == "" {
		t.Log("No test containers found to cleanup")
		return
	}

	// Stop all found containers
	stopCmd := exec.Command("docker", "stop")
	stopCmd.Stdin = strings.NewReader(containerIDs)
	if err := stopCmd.Run(); err != nil {
		t.Logf("Warning: failed to stop some test containers: %v", err)
	} else {
		t.Log("Successfully cleaned up all test containers")
	}
}

// getTestPort returns a free port for testing
func getTestPort() string {
	if port := os.Getenv("E2E_TEST_PORT"); port != "" {
		return port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "8080"
	}
	defer l.Close()
	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
}

func buildDockerImage(t *testing.T) {
	t.Log("Building Official Docker image for e2e tests...")

	cmd := exec.Command("make", "VERSION=test-e2e-official", "docker-build-official")
	cmd.Dir = "../../.." // Run this in the context of the root, where the Makefile is located.
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "expected to build Docker image successfully, output: %s", string(output))
}
