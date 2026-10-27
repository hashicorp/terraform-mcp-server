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
func TestDefaultHTTPServer(t *testing.T) {
	if !officialSDKEnabled() {
		t.Skipf("%s is false, the legacy server is under test", officialSDKEnabledEnv)
	}

	port := "8084"
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	containerID := startHTTPContainer(t, port)
	t.Cleanup(func() { stopContainer(t, containerID) })
	waitForServer(t, baseURL)

	// The old preview path is kept as an alias for clients that configured it.
	for _, path := range []string{mcpEndpoint, mcpEndpoint + "/official"} {
		t.Run(path, func(t *testing.T) {
			client := mcp.NewClient(&mcp.Implementation{Name: "e2e-default-client", Version: "0.0.1"}, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
				Endpoint:   baseURL + path,
				HTTPClient: &http.Client{Timeout: 120 * time.Second},
			}, nil)
			require.NoError(t, err, "failed to connect to %s", path)
			t.Cleanup(func() { require.NoError(t, session.Close()) })

			require.Equal(t, serverName, session.InitializeResult().ServerInfo.Name)

			tools, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err, "failed to list tools")
			names := make(map[string]bool, len(tools.Tools))
			for _, tool := range tools.Tools {
				names[tool.Name] = true
			}
			assert.True(t, names["search_modules"], "registry tools should be listed")
			assert.False(t, names["list_workspaces"], "tools that need a Terraform token should be hidden without one")

			resources, err := session.ListResources(t.Context(), nil)
			require.NoError(t, err, "failed to list resources")
			assert.Len(t, resources.Resources, 2, "the style guide and module development guide should be served")
		})
	}
}
