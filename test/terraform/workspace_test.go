package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWorkspaceReadOnlyTools(t *testing.T) {
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

	t.Run("Read workspace tags", func(t *testing.T) {
		t.Run("Happy path - valid org and workspace", func(t *testing.T) {
			tagsResult, tagsResultText := callTool(t, s, "read_workspace_tags", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     firstWorkspaceName,
			})

			require.False(t, tagsResult.IsError, "read_workspace_tags should not return an error")
			require.NotEmpty(t, tagsResultText, "read_workspace_tags result should not be empty")

			assert.Contains(t, tagsResultText, firstWorkspaceName, "Response should reference the workspace name")
		})

		t.Run("Non-existent workspace returns an error", func(t *testing.T) {
			tagsResult, _ := callTool(t, s, "read_workspace_tags", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     "this-workspace-does-not-exist-xyz",
			})

			assert.True(t, tagsResult.IsError, "read_workspace_tags should return an error for a non-existent workspace")
		})

		t.Run("Non-existent org returns an error", func(t *testing.T) {
			tagsResult, _ := callTool(t, s, "read_workspace_tags", map[string]any{
				"terraform_org_name": "this-org-does-not-exist-xyz",
				"workspace_name":     firstWorkspaceName,
			})

			assert.True(t, tagsResult.IsError, "read_workspace_tags should return an error for a non-existent org")
		})
	})

	t.Run("List workspace policy sets", func(t *testing.T) {
		t.Run("Happy path - valid org and workspace ID", func(t *testing.T) {
			policySetsResult, policySetsResultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_id":       firstWorkspaceId,
			})

			require.False(t, policySetsResult.IsError, "list_workspace_policy_sets should not return an error")
			require.NotEmpty(t, policySetsResultText, "list_workspace_policy_sets result should not be empty")

			// Response is either a JSON array of policy sets or a plain-text "No policy sets" message
			if gjson.Valid(policySetsResultText) && gjson.Parse(policySetsResultText).IsArray() {
				assert.NotEmpty(t, gjson.Get(policySetsResultText, "0.id").String(), "First policy set should have an ID")
				assert.NotEmpty(t, gjson.Get(policySetsResultText, "0.name").String(), "First policy set should have a name")
			} else {
				assert.Contains(t, policySetsResultText, firstWorkspaceId, "No-policy-sets message should reference the workspace ID")
			}
		})

		t.Run("Non-existent org returns an error", func(t *testing.T) {
			policySetsResult, _ := callTool(t, s, "list_workspace_policy_sets", map[string]any{
				"terraform_org_name": "this-org-does-not-exist-xyz",
				"workspace_id":       firstWorkspaceId,
			})

			assert.True(t, policySetsResult.IsError, "list_workspace_policy_sets should return an error for a non-existent org")
		})

	})

	t.Run("List state versions", func(t *testing.T) {
		t.Run("Happy path - workspace with state versions", func(t *testing.T) {
			svResult, svResultText := callTool(t, s, "list_state_versions", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     firstWorkspaceName,
			})

			if svResult.IsError {
				t.Log("Workspace has no state versions; skipping state version field assertions")
				return
			}

			require.NotEmpty(t, svResultText, "list_state_versions result should not be empty")
			assert.True(t, gjson.Get(svResultText, "items").IsArray(), "Response should contain an items array")
			assert.NotEmpty(t, gjson.Get(svResultText, "items.0.id").String(), "First state version should have an ID")
			assert.NotZero(t, gjson.Get(svResultText, "items.0.serial").Int(), "First state version should have a serial number")
			assert.NotEmpty(t, gjson.Get(svResultText, "items.0.terraform_version").String(), "First state version should have a terraform version")
		})

		t.Run("Non-existent workspace returns an error", func(t *testing.T) {
			svResult, _ := callTool(t, s, "list_state_versions", map[string]any{
				"terraform_org_name": firstOrgName,
				"workspace_name":     "this-workspace-does-not-exist-xyz",
			})

			assert.True(t, svResult.IsError, "list_state_versions should return an error for a non-existent workspace")
		})

		t.Run("Non-existent org returns an error", func(t *testing.T) {
			svResult, _ := callTool(t, s, "list_state_versions", map[string]any{
				"terraform_org_name": "this-org-does-not-exist-xyz",
				"workspace_name":     firstWorkspaceName,
			})

			assert.True(t, svResult.IsError, "list_state_versions should return an error for a non-existent org")
		})
	})

	t.Run("Get state version", func(t *testing.T) {
		t.Run("Neither param provided returns an error", func(t *testing.T) {
			svResult, _ := callTool(t, s, "get_state_version", map[string]any{})

			assert.True(t, svResult.IsError, "get_state_version should return an error when neither state_version_id nor workspace_id is provided")
		})

		t.Run("By workspace ID returns the latest state version", func(t *testing.T) {
			svResult, svResultText := callTool(t, s, "get_state_version", map[string]any{
				"workspace_id": firstWorkspaceId,
			})

			if svResult.IsError {
				t.Log("Workspace has no current state version; skipping state version field assertions")
				return
			}

			require.NotEmpty(t, svResultText, "get_state_version result should not be empty")
			assert.NotEmpty(t, gjson.Get(svResultText, "ID").String(), "State version should have an ID")
			assert.NotEmpty(t, gjson.Get(svResultText, "TerraformVersion").String(), "State version should include the terraform version")

			firstStateVersionID := gjson.Get(svResultText, "ID").String()

			t.Run("By state version ID returns that exact version", func(t *testing.T) {
				byIDResult, byIDResultText := callTool(t, s, "get_state_version", map[string]any{
					"state_version_id": firstStateVersionID,
				})

				require.False(t, byIDResult.IsError, "get_state_version should not return an error for a valid state_version_id")
				assert.Equal(t, firstStateVersionID, gjson.Get(byIDResultText, "ID").String(), "Returned state version ID should match the requested ID")
			})
		})

		t.Run("Non-existent state version ID returns an error", func(t *testing.T) {
			svResult, _ := callTool(t, s, "get_state_version", map[string]any{
				"state_version_id": "sv-doesnotexistxyz",
			})

			assert.True(t, svResult.IsError, "get_state_version should return an error for a non-existent state version ID")
		})

		t.Run("Non-existent workspace ID returns an error", func(t *testing.T) {
			svResult, _ := callTool(t, s, "get_state_version", map[string]any{
				"workspace_id": "ws-doesnotexistxyz",
			})

			assert.True(t, svResult.IsError, "get_state_version should return an error for a non-existent workspace ID")
		})
	})
}

