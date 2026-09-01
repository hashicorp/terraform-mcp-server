package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetLatestProviderVersionReturnsAWSVersion verifies that a common
// HashiCorp provider returns a concrete semantic version.
func TestGetLatestProviderVersionReturnsAWSVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "hashicorp",
			"name":      "aws",
		})

		require.False(t, result.IsError, "AWS provider version lookup should succeed")
		assertProviderVersion(t, text)
	})
}

// TestGetLatestProviderVersionNormalizesCase verifies that mixed-case provider
// coordinates resolve to the same registry provider.
func TestGetLatestProviderVersionNormalizesCase(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "HashiCORp",
			"name":      "AwS",
		})

		require.False(t, result.IsError, "mixed-case provider coordinates should succeed")
		assertProviderVersion(t, text)
	})
}

// TestGetLatestProviderVersionReturnsGoogleVersion verifies that the lookup
// works for another first-party provider.
func TestGetLatestProviderVersionReturnsGoogleVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "hashicorp",
			"name":      "google",
		})

		require.False(t, result.IsError, "Google provider version lookup should succeed")
		assertProviderVersion(t, text)
	})
}

// TestGetLatestProviderVersionReturnsAzureVersion verifies that another common
// HashiCorp provider returns a concrete version.
func TestGetLatestProviderVersionReturnsAzureVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "hashicorp",
			"name":      "azurerm",
		})

		require.False(t, result.IsError, "Azure provider version lookup should succeed")
		assertProviderVersion(t, text)
	})
}

// TestGetLatestProviderVersionReturnsThirdPartyVersion verifies that provider
// lookup is not limited to the HashiCorp namespace.
func TestGetLatestProviderVersionReturnsThirdPartyVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "datadog",
			"name":      "datadog",
		})

		require.False(t, result.IsError, "third-party provider version lookup should succeed")
		assertProviderVersion(t, text)
	})
}

// TestGetLatestProviderVersionRejectsMissingNamespace verifies that the
// namespace is required to identify a provider.
func TestGetLatestProviderVersionRejectsMissingNamespace(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"name": "aws",
		})

		require.True(t, result.IsError, "lookup should reject a missing namespace")
		assert.Contains(t, text, "namespace")
	})
}

// TestGetLatestProviderVersionRejectsMissingName verifies that the provider
// name is required to complete the lookup.
func TestGetLatestProviderVersionRejectsMissingName(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "hashicorp",
		})

		require.True(t, result.IsError, "lookup should reject a missing provider name")
		assert.Contains(t, text, "name")
	})
}

// TestGetLatestProviderVersionRejectsEmptyCoordinates verifies that empty
// namespace and name values are not treated as valid provider coordinates.
func TestGetLatestProviderVersionRejectsEmptyCoordinates(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": "",
			"name":      "",
		})

		require.True(t, result.IsError, "lookup should reject empty provider coordinates")
		assert.Contains(t, text, "namespace", "error should identify the first missing coordinate")
	})
}

// TestGetLatestProviderVersionRejectsUnknownProvider verifies that an unknown
// namespace and provider name return a useful lookup error.
func TestGetLatestProviderVersionRejectsUnknownProvider(t *testing.T) {
	const (
		namespace = "nonexistent-namespace"
		name      = "nonexistent-provider"
	)

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_provider_version", map[string]any{
			"namespace": namespace,
			"name":      name,
		})

		require.True(t, result.IsError, "lookup should reject an unknown provider")
		assert.Contains(t, text, "provider not found")
		assert.Contains(t, text, namespace)
		assert.Contains(t, text, name)
	})
}

// assertProviderVersion verifies that the tool returns a non-empty semantic
// version instead of an arbitrary or incomplete response.
func assertProviderVersion(t *testing.T, text string) {
	t.Helper()

	require.NotEmpty(t, text, "provider version response must not be empty")
	assert.Regexp(t, `^[0-9]+\.[0-9]+\.[0-9]+$`, text, "provider version should use semantic version format")
}
