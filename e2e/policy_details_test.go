package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetPolicyDetailsFromSearchResult verifies the documented workflow:
// search for a policy first, then fetch its details by the returned ID.
func TestGetPolicyDetailsFromSearchResult(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		policyID := searchForPolicyID(t, session, "aws")

		result, text := callTool(t, session, "get_policy_details", map[string]any{
			"terraform_policy_id": policyID,
		})

		require.False(t, result.IsError, "policy details lookup should succeed for a searched policy")
		require.NotEmpty(t, text, "policy details response must not be empty")
		assert.Contains(t, text, "## Policy details about "+policyID, "details should identify the selected policy")
		assert.Contains(t, text, "## Usage", "details should explain how to use the policy")
		assert.Contains(t, text, "policies.hcl", "details should provide the expected HCL file name")
		assert.Contains(t, text, "enforcement_level = \"advisory\"", "details should provide the policy template")
		assert.Contains(t, text, "Available policies with SHA", "details should include policy checksums")
	})
}

// TestGetPolicyDetailsRejectsMissingID verifies that the required policy ID is
// reported when the tool receives no arguments.
func TestGetPolicyDetailsRejectsMissingID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_policy_details", map[string]any{})

		require.True(t, result.IsError, "policy details should reject an empty payload")
		assert.Contains(t, text, "terraform_policy_id", "error should identify the missing policy ID")
		assert.Contains(t, text, "search_policies", "error should explain how to find a valid policy ID")
	})
}

// TestGetPolicyDetailsRejectsEmptyID verifies that an empty policy ID is
// rejected before the server makes a registry request.
func TestGetPolicyDetailsRejectsEmptyID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_policy_details", map[string]any{
			"terraform_policy_id": "",
		})

		require.True(t, result.IsError, "policy details should reject an empty policy ID")
		assert.Contains(t, text, "terraform_policy_id cannot be empty")
		assert.Contains(t, text, "search_policies")
	})
}

// TestGetPolicyDetailsRejectsMalformedID verifies that a malformed policy ID
// does not produce a successful but unusable response.
func TestGetPolicyDetailsRejectsMalformedID(t *testing.T) {
	const policyID = "malformed-policy-id"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_policy_details", map[string]any{
			"terraform_policy_id": policyID,
		})

		require.True(t, result.IsError, "policy details should reject a malformed policy ID")
		assert.Contains(t, text, "policy not found")
		assert.Contains(t, text, policyID)
		assert.Contains(t, text, "search_policies", "error should explain how to find a valid policy ID")
	})
}

// TestGetPolicyDetailsRejectsUnknownID verifies that a well-formed-looking but
// nonexistent policy ID returns a registry lookup error.
func TestGetPolicyDetailsRejectsUnknownID(t *testing.T) {
	const policyID = "policies/hashicorp/nonexistent-policy-xyz/1.0.0"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_policy_details", map[string]any{
			"terraform_policy_id": policyID,
		})

		require.True(t, result.IsError, "policy details should reject a nonexistent policy")
		assert.Contains(t, text, "policy not found")
		assert.Contains(t, text, policyID)
		assert.Contains(t, text, "search_policies", "error should explain how to find a valid policy ID")
	})
}

// searchForPolicyID obtains a real policy ID so the details test follows the
// search-before-details workflow expected by the MCP tool description.
// searchForPolicyID obtains a real ID for the details workflow.
func searchForPolicyID(t *testing.T, session *mcp.ClientSession, query string) string {
	t.Helper()

	result, text := callTool(t, session, "search_policies", map[string]any{
		"policy_query": query,
	})
	require.False(t, result.IsError, "policy search should succeed before getting details")

	ids := policyIDsFromSearchResult(t, text)
	require.NotEmpty(t, ids, "policy search should return a policy ID")
	return ids[0]
}
