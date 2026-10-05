package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: Temporarily keep TestOfficialHTTPServer while /mcp/official exists

// TestOfficialHTTPServer verifies the server name and tools exposed by the
// official SDK endpoint when the feature flag is enabled.
func TestOfficialHTTPServer(t *testing.T) {
	if !officialSDKEnabled() {
		t.Skipf("%s is not enabled", officialSDKEnabledEnv)
	}

	config := currentServerConfig()
	port := "8084"
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	containerID := startHTTPContainer(t, port, true)
	t.Cleanup(func() {
		stopContainer(t, containerID)
	})

	waitForServer(t, baseURL)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-official-client",
		Version: "0.0.1",
	}, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   baseURL + config.mcpPath,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}, nil)
	require.NoError(t, err, "failed to connect to the official HTTP server")
	t.Cleanup(func() {
		require.NoError(t, session.Close())
	})

	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	require.Equal(t, officialServerName, initResult.ServerInfo.Name)

	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err, "failed to list official server tools")

	names := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	assert.True(t, names["list_workspaces"], "official server should expose list_workspaces")
}
