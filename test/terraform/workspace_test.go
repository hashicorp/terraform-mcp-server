package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWorkspaceTools(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "list_terraform_orgs", map[string]any{})

	require.False(t, result.IsError, "Organization tool call result should not be an error")
	require.NotEmpty(t, resultText, "Organiation tool call result must not be empty")

	assert.NotEqual(t, int(gjson.Get(resultText, "items.#").Int()), 0, "Organization tool call result should not contain an empty list")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_name").String(), "Tool call result should contain organization names")
	assert.NotEmpty(t, gjson.Get(resultText, "items.0.organization_email").String(), "Tool call result should contain organization email addresses")

	firstOrgName := gjson.Get(resultText, "items.0.organization_name").String()

	listResult, listResultText := callTool(t, s, "list_workspaces",
		map[string]any{
			"terraform_org_name": firstOrgName,
		})

	require.False(t, listResult.IsError, "Workspace tool call result should not be an error")
	require.NotEmpty(t, listResultText, "Workspace tool call result should not be empty")

	assert.NotEqual(t, int(gjson.Get(listResultText, "items.#").Int()), 0, "Workspace tool call result should not contain an empty list")

	firstWorkspaceId := gjson.Get(listResultText, "items.0.id").String()
	assert.NotEmpty(t, firstWorkspaceId, "First workspace should have an ID")

	firstWorkspaceName := gjson.Get(listResultText, "items.0.workspace_name").String()
	assert.NotEmpty(t, firstWorkspaceName, "First workspace should have a name")

	t.Run("Get workspace details", func(t *testing.T) {
		t.Run("Happy path - valid org and workspace", func(t *testing.T) {
			getResult, getResultText := callTool(t, s, "get_workspace_details", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     firstWorkspaceName,
			})

			require.False(t, getResult.IsError, "get_workspace_details should not return an error")
			require.NotEmpty(t, getResultText, "get_workspace_details result should not be empty")

			assert.True(t, gjson.Get(getResultText, "data.attributes.success").Bool(), "Response should indicate success")
			assert.Equal(t, firstWorkspaceName, gjson.Get(getResultText, "data.attributes.workspace.name").String(), "Workspace name should match the requested workspace")
			assert.NotEmpty(t, gjson.Get(getResultText, "data.attributes.workspace.id").String(), "Response should include a workspace ID")
			assert.NotEmpty(t, gjson.Get(getResultText, "data.attributes.readme").String(), "Response should include a readme")
		})

		t.Run("Non-existent workspace returns an error", func(t *testing.T) {
			getResult, _ := callTool(t, s, "get_workspace_details", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     "this-workspace-does-not-exist-xyz",
			})

			assert.True(t, getResult.IsError, "get_workspace_details should return an error for a non-existent workspace")
		})

		t.Run("Non-existent org returns an error", func(t *testing.T) {
			getResult, _ := callTool(t, s, "get_workspace_details", map[string]any{
				"terraform_org_name": "this-org-does-not-exist-xyz",
				"workspace_name":     firstWorkspaceName,
			})

			assert.True(t, getResult.IsError, "get_workspace_details should return an error for a non-existent org")
		})
	})

	t.Run("List workspace variables", func(t *testing.T) {
		t.Run("Happy path - valid org and workspace", func(t *testing.T) {
			listVarsResult, listVarsResultText := callTool(t, s, "list_workspace_variables",
				map[string]any{
					"terraform_org_name": firstOrgName,
					"workspace_name":     firstWorkspaceName,
				})

			require.False(t, listVarsResult.IsError, "list_workspace_variables should not return an error")
			require.NotEmpty(t, listVarsResultText, "list_workspace_variables result should not be empty")

			assert.True(t, gjson.Get(listVarsResultText, "data").IsArray(), "list_workspace_variables result should contain a data array")

			// Check if workspace has variables, if not, then skip variable specific assert statements
			if gjson.Get(listVarsResultText, "data.#").Int() > 0 {
				assert.NotEmpty(t, gjson.Get(listVarsResultText, "data.0.id").String(), "First variable should have an ID")
				assert.Equal(t, "vars", gjson.Get(listVarsResultText, "data.0.type").String(), "First variable should have type 'vars'")
				assert.NotEmpty(t, gjson.Get(listVarsResultText, "data.0.attributes.key").String(), "First variable should have a key")
				assert.NotEmpty(t, gjson.Get(listVarsResultText, "data.0.attributes.category").String(), "First variable should have a category")
			} else {
				t.Log("Workspace has no variables; skipping per-variable field assertions")
			}
		})

		t.Run("Non-existent workspace returns an error", func(t *testing.T) {
			listVarsResult, _ := callTool(t, s, "list_workspace_variables", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     "this-workspace-does-not-exist-xyz",
			})

			assert.True(t, listVarsResult.IsError, "list_workspace_variables should return an error for a non-existent workspace")
		})

		t.Run("Non-existent org returns an error", func(t *testing.T) {
			listVarsResult, _ := callTool(t, s, "list_workspace_variables", map[string]any{
				"terraform_org_name": "this-org-does-not-exist-xyz",
				"workspace_name":     firstWorkspaceName,
			})

			assert.True(t, listVarsResult.IsError, "list_workspace_variables should return an error for a non-existent org")
		})
	})
}

