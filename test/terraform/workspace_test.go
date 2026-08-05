package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListWorkspace(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "list_terraform_orgs", map[string]any{})

	require.False(t, result.IsError, "Organization tool call result should not be an error")
	require.NotEmpty(t, resultText, "Organiation tool call result must not be empty")

	assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Organization tool call result should not contain an empty list")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_name").String(), "Tool call result should contain organization names")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_email").String(), "Tool call result should contain organization email addresses")

	orgName := gjson.Get(resultText, "items.0.organization_name").String()

	result, resultText = callTool(t, s, "list_workspaces",
	map[string]any{
		"terraform_org_name": orgName,
	})

	require.False(t, result.IsError, "Workspace tool call result should not be an error")
	require.NotEmpty(t, resultText, "Workspace tool call result should not be empty")
	
	assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Workspace tool call result should not contain an empty list")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.id").String(), "First workspace should have an ID")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.workspace_name").String(), "First workspace should have a name")
}
