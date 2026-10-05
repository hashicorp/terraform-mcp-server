package e2e

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// getTextContent joins text blocks from an MCP tool result for assertions.
func getTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}

	var text string
	for _, content := range result.Content {
		textContent, ok := content.(*mcp.TextContent)
		if ok && textContent != nil {
			text += textContent.Text
		}
	}

	return text
}

// callTool invokes one MCP tool and returns both the result and readable text.
func callTool(
	t *testing.T,
	session *mcp.ClientSession,
	toolName string,
	arguments map[string]any,
) (*mcp.CallToolResult, string) {
	t.Helper()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	require.NoError(t, err, "failed to call tool %q", toolName)

	text := getTextContent(result)
	return result, text
}
