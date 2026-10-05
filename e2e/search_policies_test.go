package e2e

import (
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var policyIDPattern = regexp.MustCompile(`(?m)^- terraform_policy_id: (policies/[^\s]+)$`)

// TestSearchPoliciesReturnsMatchingAWSPolicies verifies that a common policy
// query returns IDs and metadata needed for a later get_policy_details call.
func TestSearchPoliciesReturnsMatchingAWSPolicies(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": "aws",
		})

		require.False(t, result.IsError, "AWS policy search should succeed")
		require.NotEmpty(t, text, "AWS policy search response must not be empty")
		assertSearchPoliciesResult(t, text, "aws")
	})
}

// TestSearchPoliciesNormalizesQueryCase verifies that mixed-case input is
// normalized before matching policies.
func TestSearchPoliciesNormalizesQueryCase(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": "TeRrAfOrM",
		})

		require.False(t, result.IsError, "mixed-case policy search should succeed")
		require.NotEmpty(t, text, "mixed-case policy search response must not be empty")
		assertSearchPoliciesResult(t, text, "terraform")
	})
}

// TestSearchPoliciesMatchesTitleSubstring verifies that a query can match a
// policy title rather than only an exact policy name.
func TestSearchPoliciesMatchesTitleSubstring(t *testing.T) {
	const query = "security"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": query,
		})

		require.False(t, result.IsError, "policy title search should succeed")
		require.NotEmpty(t, text, "policy title search response must not be empty")
		assertSearchPoliciesResult(t, text, query)
		assert.Contains(t, text, "Security", "response should contain a security-related policy title")
	})
}

// TestSearchPoliciesMatchesPolicyNameWithHyphen verifies that policy names
// containing punctuation can be searched directly.
func TestSearchPoliciesMatchesPolicyNameWithHyphen(t *testing.T) {
	const query = "cis-policy"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": query,
		})

		require.False(t, result.IsError, "hyphenated policy search should succeed")
		assertSearchPoliciesResult(t, text, query)
	})
}

// TestSearchPoliciesMatchesQueryWithSpaces verifies that a multi-word policy
// title can be searched without losing the query text.
func TestSearchPoliciesMatchesQueryWithSpaces(t *testing.T) {
	const query = "Foundational Security Best Practices(FSBP)"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": query,
		})

		require.False(t, result.IsError, "policy title search with spaces should succeed")
		assertSearchPoliciesResult(t, text, query)
	})
}

// TestSearchPoliciesRejectsMissingQuery verifies that the required policy query
// is reported when the tool receives an empty payload.
func TestSearchPoliciesRejectsMissingQuery(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{})

		require.True(t, result.IsError, "search_policies should reject a missing policy_query")
		assert.Contains(t, text, "policy_query", "error should identify the missing query")
	})
}

// TestSearchPoliciesRejectsEmptyQuery verifies that an empty string is not
// treated as a request to return every policy.
func TestSearchPoliciesRejectsEmptyQuery(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": "",
		})

		require.True(t, result.IsError, "search_policies should reject an empty policy_query")
		assert.Contains(t, text, "policy_query cannot be empty")
	})
}

// TestSearchPoliciesRejectsUnknownQuery verifies that an unmatched query
// returns a useful error instead of a successful empty response.
func TestSearchPoliciesRejectsUnknownQuery(t *testing.T) {
	const query = "nonexistentpolicyxyz123"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "search_policies", map[string]any{
			"policy_query": query,
		})

		require.True(t, result.IsError, "search_policies should return an error when no policies match")
		assert.Contains(t, text, "no policies found", "error should explain that no policies matched")
		assert.Contains(t, text, query, "error should include the original query")
	})
}

// assertSearchPoliciesResult verifies the response fields needed to select a
// policy for a later get_policy_details request.
func assertSearchPoliciesResult(t *testing.T, text, query string) {
	t.Helper()

	assert.Contains(t, text, "Matching Terraform Policies", "response should identify policy search results")
	assert.Contains(t, text, "terraform_policy_id:", "response should expose a policy ID")
	assert.Contains(t, text, "Name:", "response should expose the policy name")
	assert.Contains(t, text, "Title:", "response should expose the policy title")
	assert.Contains(t, text, "Downloads:", "response should expose download counts")
	assert.Contains(t, text, "query: "+strings.ToLower(query), "response should identify the normalized search query")
	require.NotEmpty(t, policyIDsFromSearchResult(t, text), "response should contain at least one policy ID")
}

// policyIDsFromSearchResult extracts IDs that can be passed to policy details.
func policyIDsFromSearchResult(t *testing.T, text string) []string {
	t.Helper()

	matches := policyIDPattern.FindAllStringSubmatch(text, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			ids = append(ids, match[1])
		}
	}
	return ids
}
