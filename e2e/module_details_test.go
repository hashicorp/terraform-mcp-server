package e2e

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetModuleDetailsFromSearchResult verifies the documented workflow:
// search for a module first, then fetch its details by the returned ID.
func TestGetModuleDetailsFromSearchResult(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		moduleID := searchForModuleID(t, session, "aws")
		parts := splitModuleID(t, moduleID)

		result, text := callTool(t, session, "get_module_details", map[string]any{
			"module_id": moduleID,
		})

		require.False(t, result.IsError, "module details lookup should succeed for a searched module")
		require.NotEmpty(t, text, "module details response must not be empty")
		assert.Contains(t, text, "# registry://modules/"+parts[0]+"/"+parts[1], "details should identify the module")
		assert.Contains(t, text, "**Description:**", "details should include the module description")
		assert.Contains(t, text, "**Module Version:** "+parts[3], "details should include the requested module version")
		assert.Contains(t, text, "**Namespace:** "+parts[0], "details should include the module namespace")
		assert.Contains(t, text, "**Source:**", "details should include the module source")
	})
}

// TestGetModuleDetailsNormalizesModuleIDCase verifies that a valid module ID
// still works when its letters are provided in uppercase.
func TestGetModuleDetailsNormalizesModuleIDCase(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		moduleID := searchForModuleID(t, session, "aws")
		parts := splitModuleID(t, moduleID)

		result, text := callTool(t, session, "get_module_details", map[string]any{
			"module_id": strings.ToUpper(moduleID),
		})

		require.False(t, result.IsError, "module details should normalize uppercase module IDs")
		require.NotEmpty(t, text, "normalized module details response must not be empty")
		assert.Contains(t, text, "**Module Version:** "+parts[3])
		assert.Contains(t, text, "**Namespace:** "+parts[0])
	})
}

// TestGetModuleDetailsRejectsMissingID verifies that the required module ID is
// reported when the tool receives no arguments.
func TestGetModuleDetailsRejectsMissingID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_module_details", map[string]any{})

		require.True(t, result.IsError, "module details should reject an empty payload")
		assert.Contains(t, text, "module_id", "error should identify the missing module ID")
	})
}

// TestGetModuleDetailsRejectsEmptyID verifies that an empty module ID is
// rejected before the server makes a registry request.
func TestGetModuleDetailsRejectsEmptyID(t *testing.T) {
	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_module_details", map[string]any{
			"module_id": "",
		})

		require.True(t, result.IsError, "module details should reject an empty module ID")
		assert.Contains(t, text, "module_id cannot be empty")
	})
}

// TestGetModuleDetailsRejectsInvalidFormat verifies that IDs without the four
// required namespace/name/provider/version parts are rejected.
func TestGetModuleDetailsRejectsInvalidFormat(t *testing.T) {
	const moduleID = "invalid-format"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_module_details", map[string]any{
			"module_id": moduleID,
		})

		require.True(t, result.IsError, "module details should reject an invalid module ID format")
		assert.Contains(t, text, "invalid module ID format")
		assert.Contains(t, text, "namespace/name/provider/version")
	})
}

// TestGetModuleDetailsRejectsUnknownModule verifies that a correctly shaped
// but nonexistent module ID returns a registry lookup error.
func TestGetModuleDetailsRejectsUnknownModule(t *testing.T) {
	const moduleID = "hashicorp/nonexistentmodule/aws/1.0.0"

	runForEachTransport(t, func(t *testing.T, session *mcp.ClientSession) {
		result, text := callTool(t, session, "get_module_details", map[string]any{
			"module_id": moduleID,
		})

		require.True(t, result.IsError, "module details should reject a nonexistent module")
		assert.Contains(t, text, "module not found")
		assert.Contains(t, text, "use search_modules", "error should explain how to find a valid module ID")
		assert.Contains(t, text, moduleID)
	})
}

// searchForModuleID obtains a real module ID so the details test follows the
// same search-before-details workflow expected by the MCP tool description.
func searchForModuleID(t *testing.T, session *mcp.ClientSession, query string) string {
	t.Helper()

	result, text := callTool(t, session, "search_modules", map[string]any{
		"module_query": query,
	})
	require.False(t, result.IsError, "module search should succeed before getting details")

	ids := moduleIDsFromSearchResult(t, text)
	require.NotEmpty(t, ids, "module search should return a module ID")
	return ids[0]
}

// splitModuleID validates and separates the ID fields used by assertions.
func splitModuleID(t *testing.T, moduleID string) []string {
	t.Helper()

	parts := strings.Split(moduleID, "/")
	require.Len(t, parts, 4, "module ID should have namespace/name/provider/version format")
	return parts
}
