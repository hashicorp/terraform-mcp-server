package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetLatestModuleVersionReturnsAWSVersion verifies that a known AWS module
// returns a concrete semantic version from the registry.
func TestGetLatestModuleVersionReturnsAWSVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
			"module_provider":  "aws",
		})

		require.False(t, result.IsError, "AWS module version lookup should succeed")
		assertModuleVersion(t, text)
	})
}

// TestGetLatestModuleVersionNormalizesCase verifies that mixed-case module
// coordinates resolve to the same version as lowercase coordinates.
func TestGetLatestModuleVersionNormalizesCase(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "TerraFORM-AwS-ModuLES",
			"module_name":      "VpC",
			"module_provider":  "AWs",
		})

		require.False(t, result.IsError, "mixed-case module coordinates should succeed")
		assertModuleVersion(t, text)
	})
}

// TestGetLatestModuleVersionReturnsGoogleVersion verifies that the lookup works
// for a provider ecosystem other than AWS.
func TestGetLatestModuleVersionReturnsGoogleVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "terraform-google-modules",
			"module_name":      "network",
			"module_provider":  "google",
		})

		require.False(t, result.IsError, "Google module version lookup should succeed")
		assertModuleVersion(t, text)
	})
}

// TestGetLatestModuleVersionReturnsHashiCorpVersion verifies that a module
// published directly by HashiCorp is supported.
func TestGetLatestModuleVersionReturnsHashiCorpVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "hashicorp",
			"module_name":      "consul",
			"module_provider":  "aws",
		})

		require.False(t, result.IsError, "HashiCorp module version lookup should succeed")
		assertModuleVersion(t, text)
	})
}

// TestGetLatestModuleVersionReturnsAzureVersion verifies that Azure module
// publisher names are handled correctly.
func TestGetLatestModuleVersionReturnsAzureVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "Azure",
			"module_name":      "network",
			"module_provider":  "azurerm",
		})

		require.False(t, result.IsError, "Azure module version lookup should succeed")
		assertModuleVersion(t, text)
	})
}

// TestGetLatestModuleVersionRejectsMissingPublisher verifies that the publisher
// is required to identify a module in the registry.
func TestGetLatestModuleVersionRejectsMissingPublisher(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_name":     "vpc",
			"module_provider": "aws",
		})

		require.True(t, result.IsError, "lookup should reject a missing module publisher")
		assert.Contains(t, text, "module_publisher")
	})
}

// TestGetLatestModuleVersionRejectsMissingName verifies that the module name is
// required to identify a registry module.
func TestGetLatestModuleVersionRejectsMissingName(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "terraform-aws-modules",
			"module_provider":  "aws",
		})

		require.True(t, result.IsError, "lookup should reject a missing module name")
		assert.Contains(t, text, "module_name")
	})
}

// TestGetLatestModuleVersionRejectsMissingProvider verifies that the provider
// is required to complete the module registry path.
func TestGetLatestModuleVersionRejectsMissingProvider(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
		})

		require.True(t, result.IsError, "lookup should reject a missing module provider")
		assert.Contains(t, text, "module_provider")
	})
}

// TestGetLatestModuleVersionRejectsUnknownModule verifies that an unknown
// module returns an error containing the requested module coordinates.
func TestGetLatestModuleVersionRejectsUnknownModule(t *testing.T) {
	const (
		publisher = "nonexistent-publisher"
		name      = "nonexistent-module"
		provider  = "nonexistent-provider"
	)

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_latest_module_version", map[string]any{
			"module_publisher": publisher,
			"module_name":      name,
			"module_provider":  provider,
		})

		require.True(t, result.IsError, "lookup should reject an unknown module")
		assert.Contains(t, text, publisher)
		assert.Contains(t, text, name)
		assert.Contains(t, text, provider)
	})
}

// assertModuleVersion verifies the response format returned by the version
// lookup and prevents an empty or non-version response from passing.
func assertModuleVersion(t *testing.T, text string) {
	t.Helper()

	require.NotEmpty(t, text, "module version response must not be empty")
	assert.Regexp(t, `^[0-9]+\.[0-9]+\.[0-9]+$`, text, "module version should use semantic version format")
}
