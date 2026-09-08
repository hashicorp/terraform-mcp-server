package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchProvidersRequiredValuesResource verifies that the tool finds a resource
// when the provider name, namespace, and service slug are supplied.
func TestSearchProvidersRequiredValuesResource(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":      "dns",
			"provider_namespace": "hashicorp",
			"service_slug":       "ns_record_set",
		})

		require.False(t, result.IsError)
		require.NotEmpty(t, text)
		assertSearchProvidersResult(t, text, "hashicorp/dns", "resources", "ns_record_set")
	})
}

// TestSearchProvidersDataSourceWithProviderNamePrefix verifies that a prefixed
// data-source query returns a data-source document rather than a resource.
func TestSearchProvidersDataSourceWithProviderNamePrefix(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "dns",
			"provider_namespace":     "hashicorp",
			"provider_document_type": "data-sources",
			"service_slug":           "dns_ns_record_set",
		})

		require.False(t, result.IsError, "data-source search should not return an error")
		require.NotEmpty(t, text, "data-source search response must not be empty")
		assertSearchProvidersResult(t, text, "hashicorp/dns", "data-sources", "ns_record_set")
	})
}

// TestSearchProvidersDefaultsHashicorpNamespaceAndLatestVersion verifies that
// HashiCorp providers work when namespace and version are omitted, and that the
// server resolves latest to a concrete version.
func TestSearchProvidersDefaultsHashicorpNamespaceAndLatestVersion(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name": "aws",
			"service_slug":  "aws_s3_bucket",
		})

		require.False(t, result.IsError, "search with omitted namespace and version should succeed for HashiCorp providers")
		require.NotEmpty(t, text, "defaulted provider search response must not be empty")
		assertSearchProvidersResult(t, text, "hashicorp/aws", "resources", "s3_bucket")
		assert.Regexp(t, `Terraform provider hashicorp/aws version: [0-9]+\.[0-9]+\.[0-9]+`, text)
		assert.NotContains(t, text, "version: latest", "the server should resolve latest to a concrete provider version")
	})
}

// TestSearchProvidersRejectsMissingServiceSlug verifies that the tool reports
// a useful error when the required search query is missing.
func TestSearchProvidersRejectsMissingServiceSlug(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":      "google",
			"provider_namespace": "hashicorp",
		})

		require.True(t, result.IsError, "search without service_slug should return a tool error")
		assert.Contains(t, text, "service_slug", "error should identify the missing service_slug argument")
	})
}

// TestSearchProvidersRejectsUnknownProvider verifies that an unknown provider
// produces a tool error that identifies the failed provider lookup.
func TestSearchProvidersRejectsUnknownProvider(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":      "vaults",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
			"service_slug":       "vaults",
		})

		require.True(t, result.IsError, "search for an unknown provider should return a tool error")
		assert.Contains(t, text, "vaults", "error should identify the unknown provider")
		assert.Contains(t, text, "provider", "error should explain that provider resolution failed")
	})
}

// TestSearchProvidersThirdPartyProvider verifies that provider resolution also
// works for a provider published outside the HashiCorp namespace.
func TestSearchProvidersThirdPartyProvider(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "pinecone",
			"provider_namespace":     "pinecone-io",
			"provider_version":       "latest",
			"provider_document_type": "resources",
			"service_slug":           "pinecone_index",
		})

		require.False(t, result.IsError, "third-party provider search should not return an error")
		require.NotEmpty(t, text, "third-party provider response must not be empty")
		assertSearchProvidersResult(t, text, "pinecone-io/pinecone", "resources", "index")
	})
}

// TestSearchProvidersThirdPartyDataSource verifies data-source lookup for a
// provider outside the HashiCorp namespace.
func TestSearchProvidersThirdPartyDataSource(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "terracurl",
			"provider_namespace":     "devops-rob",
			"provider_document_type": "data-sources",
			"service_slug":           "terracurl",
		})

		require.False(t, result.IsError, "third-party data-source search should succeed")
		require.NotEmpty(t, text, "third-party data-source response must not be empty")
		assertSearchProvidersResult(t, text, "devops-rob/terracurl", "data-sources", "request")
	})
}

// TestSearchProvidersGuidesDocumentation verifies the v2 provider guides path.
func TestSearchProvidersGuidesDocumentation(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "guides",
			"service_slug":           "custom-service-endpoints",
		})

		require.False(t, result.IsError, "provider guides search should succeed")
		require.NotEmpty(t, text, "provider guides response must not be empty")
		assert.Contains(t, text, "for guides in Terraform provider hashicorp/aws")
		assert.Contains(t, text, "providerDocID:")
	})
}

// TestSearchProvidersFunctionsDocumentation verifies the v2 provider functions path.
func TestSearchProvidersFunctionsDocumentation(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "google",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "functions",
			"service_slug":           "name_from_id",
		})

		require.False(t, result.IsError, "provider functions search should succeed")
		require.NotEmpty(t, text, "provider functions response must not be empty")
		assert.Contains(t, text, "for functions in Terraform provider hashicorp/google")
		assert.Contains(t, text, "providerDocID:")
	})
}

// TestSearchProvidersOverviewDocumentation verifies the v2 provider overview path.
func TestSearchProvidersOverviewDocumentation(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "google",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "overview",
			"service_slug":           "index",
		})

		require.False(t, result.IsError, "provider overview search should succeed")
		require.NotEmpty(t, text, "provider overview response must not be empty")
		assert.Contains(t, text, "google", "overview response should identify the provider")
	})
}

// TestSearchProvidersActionsDocumentation verifies the v2 provider actions path.
func TestSearchProvidersActionsDocumentation(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "actions",
			"service_slug":           "ec2",
		})

		require.False(t, result.IsError, "provider actions search should succeed")
		require.NotEmpty(t, text, "provider actions response must not be empty")
		assert.Contains(t, text, "for actions in Terraform provider hashicorp/aws")
		assert.Contains(t, text, "providerDocID:")
	})
}

// TestSearchProvidersListResourcesDocumentation verifies the v2 list-resources path.
func TestSearchProvidersListResourcesDocumentation(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "list-resources",
			"service_slug":           "instance",
		})

		require.False(t, result.IsError, "provider list-resources search should succeed")
		require.NotEmpty(t, text, "provider list-resources response must not be empty")
		assert.Contains(t, text, "for list-resources in Terraform provider hashicorp/aws")
		assert.Contains(t, text, "providerDocID:")
	})
}

// TestSearchProvidersFallsBackFromMalformedNamespace verifies that a failed
// namespace lookup falls back to the HashiCorp namespace when the provider is
// available there.
func TestSearchProvidersFallsBackFromMalformedNamespace(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_providers", map[string]any{
			"provider_name":      "vault",
			"provider_namespace": "hashicorp-malformed",
			"provider_version":   "latest",
			"service_slug":       "vault_aws_auth_backend_role",
		})

		require.False(t, result.IsError, "search should fall back to the HashiCorp namespace")
		require.NotEmpty(t, text, "fallback provider response must not be empty")
		assert.Contains(t, text, "Terraform provider hashicorp/vault")
		assert.Contains(t, text, "Category: resources")
	})
}

// assertSearchProvidersResult checks the response fields needed by callers to
// choose a document for a later get_provider_details request.
func assertSearchProvidersResult(
	t *testing.T,
	text string,
	provider string,
	category string,
	expectedTitle string,
) {
	t.Helper()

	assert.Contains(t, text, "providerDocID:", "response should expose a provider document ID")
	assert.Contains(t, text, "Title:", "response should expose a document title")
	assert.Contains(t, text, "Category: "+category, "response should contain the requested document category")
	assert.Contains(t, text, "- Title: "+expectedTitle, "response should contain the selected document title")
	assert.Contains(t, text, "Terraform provider "+provider, "response should identify the resolved provider")
}
