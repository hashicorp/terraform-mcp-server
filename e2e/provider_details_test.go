package e2e

import (
	"regexp"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var providerDocIDPattern = regexp.MustCompile(`(?m)^- providerDocID: ([0-9]+)$`)

// TestGetProviderDetailsFromSearchResult verifies the documented workflow:
// search for a provider document first, then fetch its full content by ID.
func TestGetProviderDetailsFromSearchResult(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		searchResult, searchText := callTool(t, session, "search_providers", map[string]any{
			"provider_name":          "dns",
			"provider_namespace":     "hashicorp",
			"provider_document_type": "resources",
			"service_slug":           "ns_record_set",
		})

		require.False(t, searchResult.IsError, "provider search should succeed before getting details")
		providerDocID := providerDocIDFromSearchResult(t, searchText)

		detailsResult, detailsText := callTool(t, session, "get_provider_details", map[string]any{
			"provider_doc_id": providerDocID,
		})

		require.False(t, detailsResult.IsError, "provider details lookup should succeed for a searched document")
		require.NotEmpty(t, detailsText, "provider details response must not be empty")
		assert.Contains(t, detailsText, "dns_ns_record_set", "details should describe the searched DNS resource")
		assert.Contains(t, detailsText, "resource", "details should contain Terraform resource documentation")
	})
}

// TestGetProviderDetailsRejectsMissingID verifies that the required document ID
// is reported when the tool receives no arguments.
func TestGetProviderDetailsRejectsMissingID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_provider_details", map[string]any{})

		require.True(t, result.IsError, "provider details should reject an empty payload")
		assert.Contains(t, text, "provider_doc_id", "error should identify the missing document ID")
	})
}

// TestGetProviderDetailsRejectsEmptyID verifies that an empty document ID is
// rejected before the server makes a registry request.
func TestGetProviderDetailsRejectsEmptyID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_provider_details", map[string]any{
			"provider_doc_id": "",
		})

		require.True(t, result.IsError, "provider details should reject an empty document ID")
		assert.Contains(t, text, "provider_doc_id cannot be empty")
	})
}

// TestGetProviderDetailsRejectsNonnumericID verifies that the tool accepts only
// numeric tfprovider-compatible document IDs.
func TestGetProviderDetailsRejectsNonnumericID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_provider_details", map[string]any{
			"provider_doc_id": "invalid-doc-id",
		})

		require.True(t, result.IsError, "provider details should reject a nonnumeric document ID")
		assert.Contains(t, text, "must be a valid number")
		assert.Contains(t, text, "search_providers", "error should explain how to find a valid document ID")
	})
}

// TestGetProviderDetailsRejectsUnknownNumericID verifies that numeric format
// validation does not incorrectly accept a document that is absent from the registry.
func TestGetProviderDetailsRejectsUnknownNumericID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		const providerDocID = "3356809"

		result, text := callTool(t, session, "get_provider_details", map[string]any{
			"provider_doc_id": providerDocID,
		})

		require.True(t, result.IsError, "provider details should reject an unknown numeric document ID")
		assert.Contains(t, text, "provider doc not found")
		assert.Contains(t, text, providerDocID, "error should identify the missing document ID")
	})
}

// providerDocIDFromSearchResult extracts the ID required by provider details.
func providerDocIDFromSearchResult(t *testing.T, text string) string {
	t.Helper()

	matches := providerDocIDPattern.FindStringSubmatch(text)
	require.Len(t, matches, 2, "search response should contain one provider document ID")
	return matches[1]
}
