package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetProviderCapabilitiesReturnsAWSResources verifies that the tool
// summarizes documentation capabilities from a real provider registry response.
func TestGetProviderCapabilitiesReturnsAWSResources(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_provider_capabilities", map[string]any{
			"namespace": "hashicorp",
			"name":      "aws",
			"version":   "latest",
		})

		require.False(t, result.IsError, "AWS provider capability lookup should succeed")
		require.NotEmpty(t, text, "provider capability response must not be empty")
		assert.Regexp(t, `Provider Capabilities: hashicorp/aws \(v[0-9]+\.[0-9]+\.[0-9]+\)`, text)
		assert.Contains(t, text, "Resources:", "AWS provider should expose resource documentation")
	})
}
