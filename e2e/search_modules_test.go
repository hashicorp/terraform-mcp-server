package e2e

import (
	"regexp"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var moduleIDPattern = regexp.MustCompile(`(?m)^- module_id: ([^\s]+)$`)

// TestSearchModulesReturnsMatchingAWSModules verifies that a common module
// query returns usable module records with the fields callers need.
func TestSearchModulesReturnsMatchingAWSModules(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": "aws",
		})

		require.False(t, result.IsError, "AWS module search should succeed")
		require.NotEmpty(t, text, "AWS module search response must not be empty")
		assertSearchModulesResult(t, text, "aws")
	})
}

// TestSearchModulesNormalizesQueryCase verifies that users can search with a
// capitalized provider name and still receive matching module results.
func TestSearchModulesNormalizesQueryCase(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": "vSphere",
		})

		require.False(t, result.IsError, "case-insensitive module search should succeed")
		require.NotEmpty(t, text, "case-insensitive module search response must not be empty")
		assertSearchModulesResult(t, text, "vsphere")
	})
}

// TestSearchModulesReturnsAllModulesForEmptyQuery verifies the supported
// behavior of listing modules when no search term is provided.
func TestSearchModulesReturnsAllModulesForEmptyQuery(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": "",
		})

		require.False(t, result.IsError, "an empty module query should list modules")
		require.NotEmpty(t, text, "empty-query module response must not be empty")
		assertSearchModulesResult(t, text, "")
	})
}

// TestSearchModulesFindsAviatrixModules verifies support for a provider whose
// registry module names use the terraform-provider-modules convention.
func TestSearchModulesFindsAviatrixModules(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": "aviatrix",
		})

		require.False(t, result.IsError, "Aviatrix module search should succeed")
		assertSearchModulesResult(t, text, "aviatrix")
	})
}

// TestSearchModulesFindsOCIModules verifies that OCI module searches return
// normal module records.
func TestSearchModulesFindsOCIModules(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": "oci",
		})

		require.False(t, result.IsError, "OCI module search should succeed")
		assertSearchModulesResult(t, text, "oci")
	})
}

// TestSearchModulesSupportsQueriesWithSpaces verifies that a multi-word query
// is passed to the registry and returns usable results.
func TestSearchModulesSupportsQueriesWithSpaces(t *testing.T) {
	const query = "vertex ai"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": query,
		})

		require.False(t, result.IsError, "module search with spaces should succeed")
		assertSearchModulesResult(t, text, query)
	})
}

// TestSearchModulesSupportsPagination verifies that the current_offset input
// is accepted and produces a different page of module results.
func TestSearchModulesSupportsPagination(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		firstPageResult, firstPageText := callTool(t, session, "search_modules", map[string]any{
			"module_query":   "aws",
			"current_offset": 0,
		})
		require.False(t, firstPageResult.IsError, "first module page should succeed")
		firstPageIDs := moduleIDsFromSearchResult(t, firstPageText)

		nextPageResult, nextPageText := callTool(t, session, "search_modules", map[string]any{
			"module_query":   "aws",
			"current_offset": 5,
		})
		require.False(t, nextPageResult.IsError, "offset module page should succeed")
		nextPageIDs := moduleIDsFromSearchResult(t, nextPageText)

		require.NotEmpty(t, firstPageIDs, "first page should contain module IDs")
		require.NotEmpty(t, nextPageIDs, "offset page should contain module IDs")
		assert.NotEqual(t, firstPageIDs[0], nextPageIDs[0], "changing the offset should move to a different result page")
	})
}

// TestSearchModulesRejectsMissingQuery verifies that the required module query
// is reported when the tool receives an empty payload.
func TestSearchModulesRejectsMissingQuery(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{})

		require.True(t, result.IsError, "search_modules should reject a missing module_query")
		assert.Contains(t, text, "module_query", "error should identify the missing query")
	})
}

// TestSearchModulesRejectsUnknownQuery verifies that a query with no registry
// matches returns a useful error instead of an empty successful response.
func TestSearchModulesRejectsUnknownQuery(t *testing.T) {
	const query = "unknownprovider"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_modules", map[string]any{
			"module_query": query,
		})

		require.True(t, result.IsError, "search_modules should return an error when no modules match")
		assert.Contains(t, text, "failed to parse module results", "error should explain that the query did not produce usable results")
		assert.Contains(t, text, query, "error should include the original query")
	})
}

// assertSearchModulesResult verifies the response shape used to select a
// module for a later get_module_details request.
func assertSearchModulesResult(t *testing.T, text, query string) {
	t.Helper()

	assert.Contains(t, text, "Available Terraform Modules", "response should identify module search results")
	assert.Contains(t, text, "module_id:", "response should expose a module ID")
	assert.Contains(t, text, "Name:", "response should expose a module name")
	assert.Contains(t, text, "Description:", "response should expose a module description")
	assert.Contains(t, text, "Downloads:", "response should expose download counts")
	assert.Contains(t, text, "Verified:", "response should expose verification status")
	assert.Contains(t, text, "Published:", "response should expose the publication date")
	assert.Contains(t, text, "for "+query, "response should identify the normalized search query")
	require.NotEmpty(t, moduleIDsFromSearchResult(t, text), "response should contain at least one module ID")
}

// moduleIDsFromSearchResult extracts IDs that can be passed to module details.
func moduleIDsFromSearchResult(t *testing.T, text string) []string {
	t.Helper()

	matches := moduleIDPattern.FindAllStringSubmatch(text, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			ids = append(ids, match[1])
		}
	}
	return ids
}
