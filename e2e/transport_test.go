package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const (
	officialSDKEnabledEnv = "TF_X_OFFICIAL_SDK_ENABLED"
	legacyServerName      = "terraform-mcp-server"
	officialServerName    = "terraform-mcp-official"
	legacyMCPEndpoint     = "/mcp"
	officialMCPEndpoint   = "/mcp/official"
)

type e2eServerConfig struct {
	officialSDK bool
	serverName  string
	mcpPath     string
}

func officialSDKEnabled() bool {
	return os.Getenv(officialSDKEnabledEnv) == "true"
}

// currentServerConfig selects the HTTP endpoint and server name for this run.
func currentServerConfig() e2eServerConfig {
	if officialSDKEnabled() {
		return e2eServerConfig{
			officialSDK: true,
			serverName:  officialServerName,
			mcpPath:     officialMCPEndpoint,
		}
	}

	return e2eServerConfig{
		officialSDK: false,
		serverName:  legacyServerName,
		mcpPath:     legacyMCPEndpoint,
	}
}

func legacyServerConfig() e2eServerConfig {
	return e2eServerConfig{
		serverName: legacyServerName,
		mcpPath:    legacyMCPEndpoint,
	}
}

var e2eTransports = []struct {
	name    string
	factory func(*testing.T) (*mcp.ClientSession, func())
}{
	{
		name:    "Stdio",
		factory: createStdioClient,
	},
	{
		name:    "HTTP",
		factory: createHTTPClient,
	},
}

// runForEachTransport repeats one tool test over isolated Stdio and HTTP sessions.
func runForEachTransport(
	t *testing.T,
	test func(*testing.T, *mcp.ClientSession),
) {
	t.Helper()

	for _, transport := range e2eTransports {
		transport := transport

		t.Run(transport.name, func(t *testing.T) {
			// A new session prevents one test from sharing MCP state with another.
			session, cleanup := transport.factory(t)
			t.Cleanup(cleanup)

			test(t, session)
		})
	}
}

// createStdioClient starts the image as a process connected to MCP Stdio.
func createStdioClient(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()
	t.Log("Starting Stdio MCP client...")

	cmd := exec.Command(
		"docker",
		"run",
		"-i",
		"--rm",
		"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
		"-e", "MCP_RATE_LIMIT_SESSION=50:100",
		"terraform-mcp-server:test-e2e",
	)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Connect starts the Docker command and performs MCP initialization.
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command: cmd,
	}, nil)
	require.NoError(t, err, "failed to connect over stdio")
	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	require.Equal(t, legacyServerName, initResult.ServerInfo.Name)

	cleanup := func() {
		if err := session.Close(); err != nil {
			t.Logf("failed to close stdio session: %v", err)
		}
	}

	return session, cleanup
}

// createHTTPClient starts an HTTP container and connects with the official SDK.
func createHTTPClient(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()
	t.Log("Starting HTTP MCP server...")

	port := getTestPort()
	// Registry tools are registered on the legacy endpoint today. The official
	// endpoint currently exposes only list_workspaces.
	config := legacyServerConfig()
	baseURL := fmt.Sprintf("http://localhost:%s", port)
	mcpURL := baseURL + config.mcpPath

	containerID := startHTTPContainer(t, port, officialSDKEnabled())
	// Register immediately so failures during readiness or Connect
	// still stop the container.
	t.Cleanup(func() {
		stopContainer(t, containerID)
	})

	// Wait for the health endpoint before opening the MCP connection.
	waitForServer(t, baseURL)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "0.0.1",
	}, nil)

	httpClient := &http.Client{
		Timeout:   120 * time.Second,
		Transport: http.DefaultTransport,
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   mcpURL,
		HTTPClient: httpClient,
	}, nil)
	require.NoError(t, err, "failed to connect over HTTP")
	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	require.Equal(t, config.serverName, initResult.ServerInfo.Name)

	cleanup := func() {
		// Close MCP before the container cleanup registered above runs.
		if err := session.Close(); err != nil {
			t.Logf("failed to close HTTP session: %v", err)
		}
	}

	return session, cleanup
}

// getTestPort returns the configured HTTP port or the local default.
func getTestPort() string {
	if port := os.Getenv("E2E_TEST_PORT"); port != "" {
		return port
	}
	return "8080"
}
