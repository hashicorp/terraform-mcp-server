package e2e

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startHTTPContainer starts the server in stateful streamable HTTP mode.
func startHTTPContainer(t *testing.T, port string, officialSDK bool) string {
	t.Helper()

	// Map the host test port to the server's container port.
	portMapping := fmt.Sprintf("%s:8080", port)

	sdkEnabled := "false"
	if officialSDK {
		sdkEnabled = "true"
	}

	cmd := exec.Command(
		"docker", "run",
		"-d",
		"--rm",
		"-e", "TRANSPORT_MODE=streamable-http",
		"-e", "TRANSPORT_HOST=0.0.0.0",
		"-e", "MCP_SESSION_MODE=stateful",
		"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
		"-e", "MCP_RATE_LIMIT_SESSION=50:100",
		"-e", officialSDKEnabledEnv+"="+sdkEnabled,
		"-p", portMapping,
		"terraform-mcp-server:test-e2e",
	)

	output, err := cmd.CombinedOutput()
	require.NoError(
		t,
		err,
		"failed to start HTTP container: %s",
		string(output),
	)

	containerID := strings.TrimSpace(string(output))
	require.NotEmpty(t, containerID, "Docker did not return a container ID")

	t.Logf("Started HTTP container %s on port %s", containerID, port)

	return containerID
}

// waitForServer polls health until the HTTP server accepts requests.
func waitForServer(t *testing.T, baseURL string) {
	t.Helper()

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	healthURL := baseURL + "/health"

	// Startup can take a few seconds while the container initializes.
	for attempt := 0; attempt < 30; attempt++ {
		resp, err := client.Get(healthURL)

		if resp != nil {
			resp.Body.Close()
		}

		if err == nil && resp.StatusCode == http.StatusOK {
			t.Log("HTTP server is ready")
			return
		}

		select {
		case <-t.Context().Done():
			t.Fatalf("context canceled while waiting for HTTP server: %v", t.Context().Err())
		case <-time.After(time.Second):
		}
	}

	t.Fatalf("HTTP server failed to start within 30 seconds: %s", healthURL)
}

// stopContainer stops one test container and falls back to kill if needed.
func stopContainer(t *testing.T, containerID string) {
	t.Helper()

	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return
	}

	t.Logf("Stopping HTTP container %s", containerID)

	stopCmd := exec.Command("docker", "stop", containerID)
	if output, err := stopCmd.CombinedOutput(); err == nil {
		t.Logf("Stopped HTTP container %s", containerID)
		return
	} else {
		t.Logf(
			"docker stop failed for %s: %v: %s",
			containerID,
			err,
			strings.TrimSpace(string(output)),
		)
	}

	// Force removal if graceful shutdown failed.
	killCmd := exec.Command("docker", "kill", containerID)
	if output, err := killCmd.CombinedOutput(); err != nil {
		t.Logf(
			"docker kill failed for %s: %v: %s",
			containerID,
			err,
			strings.TrimSpace(string(output)),
		)
	}
}

// cleanupTestContainers stops any test containers left after package execution.
func cleanupTestContainers() {
	const image = "terraform-mcp-server:test-e2e"

	output, err := exec.Command(
		"docker",
		"ps",
		"-q",
		"--filter", "ancestor="+image,
	).CombinedOutput()
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"failed to list E2E containers: %v\n%s\n",
			err,
			output,
		)
		return
	}

	containerIDs := strings.Fields(string(output))
	if len(containerIDs) == 0 {
		return
	}

	// Stop all matching containers in one Docker command.
	args := append([]string{"stop"}, containerIDs...)
	output, err = exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"failed to stop E2E containers: %v\n%s\n",
			err,
			output,
		)
	}
}

// startHTTPContainerWithCORS starts an HTTP server with a selected CORS policy.
func startHTTPContainerWithCORS(
	t *testing.T,
	port string,
	mode string,
	origins string,
	officialSDK bool,
) string {
	t.Helper()

	portMapping := fmt.Sprintf("%s:8080", port)

	sdkEnabled := "false"
	if officialSDK {
		sdkEnabled = "true"
	}

	cmd := exec.Command(
		"docker", "run",
		"-d",
		"--rm",
		"-e", "TRANSPORT_MODE=streamable-http",
		"-e", "TRANSPORT_HOST=0.0.0.0",
		"-e", "MCP_SESSION_MODE=stateful",
		"-e", "MCP_RATE_LIMIT_GLOBAL=50:100",
		"-e", "MCP_RATE_LIMIT_SESSION=50:100",
		"-e", officialSDKEnabledEnv+"="+sdkEnabled,
		"-e", "MCP_CORS_MODE="+mode,
		"-e", "MCP_ALLOWED_ORIGINS="+origins,
		"-p", portMapping,
		"terraform-mcp-server:test-e2e",
	)

	output, err := cmd.CombinedOutput()
	require.NoError(
		t,
		err,
		"failed to start CORS container: %s",
		string(output),
	)

	containerID := strings.TrimSpace(string(output))
	require.NotEmpty(t, containerID, "Docker did not return a container ID")

	t.Logf(
		"Started CORS container %s on port %s with mode %s and origins %s",
		containerID,
		port,
		mode,
		origins,
	)

	return containerID
}
