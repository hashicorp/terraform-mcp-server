package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const (
	officialSDKEnabledEnv = "TF_X_OFFICIAL_SDK_ENABLED"
	serverName            = "terraform-mcp-server"
	mcpEndpoint           = "/mcp"
)

type e2eServerConfig struct {
	serverName string
	mcpPath    string
}

// officialSDKEnabled reports which server this run tests. The official go-sdk
// server is the default, set TF_X_OFFICIAL_SDK_ENABLED=false to test the legacy
// mark3labs fallback.
func officialSDKEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv(officialSDKEnabledEnv))
	return err != nil || enabled
}

// sdkEnvArgs returns the docker flags that select the server. The official
// server is the default, so no flag is passed for it.
func sdkEnvArgs() []string {
	if officialSDKEnabled() {
		return nil
	}
	return []string{"-e", officialSDKEnabledEnv + "=false"}
}

// currentServerConfig returns the endpoint and server name for this run. Both
// servers use the same ones.
func currentServerConfig() e2eServerConfig {
	return e2eServerConfig{serverName: serverName, mcpPath: mcpEndpoint}
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

// Builds the docker command that runs the server over stdio.
// Docker passes everything after the image name to the server inside the
// container, so every -e flag has to come BEFORE the image name.
func stdioDockerCommand() *exec.Cmd {
	args := []string{
		"run",
		"-i",
		"--rm",
		"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
		"-e", "MCP_RATE_LIMIT_SESSION=50:100",
	}
	args = append(args, sdkEnvArgs()...)
	args = append(args, "terraform-mcp-server:test-e2e")
	return exec.Command("docker", args...)
}

// createStdioClient starts the image as a process connected to MCP Stdio.
func createStdioClient(t *testing.T) (*mcp.ClientSession, func()) {
	t.Helper()
	t.Log("Starting Stdio MCP client...")
	cmd := stdioDockerCommand()
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
	require.Equal(t, currentServerConfig().serverName, initResult.ServerInfo.Name)

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
	// Select the official endpoint when the SDK feature flag is enabled.
	config := currentServerConfig()
	baseURL := fmt.Sprintf("http://localhost:%s", port)
	mcpURL := baseURL + config.mcpPath

	containerID := startHTTPContainer(t, port)
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
