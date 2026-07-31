package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListOrganizations(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "list_terraform_orgs", map[string]any{})

	require.False(t, result.IsError, "Tool call result should not be an error")
	require.NotEmpty(t, resultText, "Tool call result must not be empty")

	assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Tool call result should not contain an empty list")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_name").String(), "Tool call result should contain organization names")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_email").String(), "Tool call result should contain organization email addresses")
}
