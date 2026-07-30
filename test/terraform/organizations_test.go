package terraform

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListOrganziations(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	result, err := s.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "list_terraform_orgs",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("Failed to call tool: %v", err)
	}

	resultText := getTextContent(result)

	require.False(t, result.IsError, "Tool call result should not be an error")
	require.NotEmpty(t, resultText, "Tool call result must not be empty")

	assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Tool call result should not contain an empty list")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_name").String(), "Tool call result should contain organization names")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_email").String(), "Tool call result should contain organization email addresses")
}
