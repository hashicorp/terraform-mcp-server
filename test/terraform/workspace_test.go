package terraform

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWorkspaceHappyPath(t *testing.T) {
	requireTfOperations(t)
	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	wsName := randomName("workspace-")

	// Create workspace
	createResult, createResultText := callTool(t, s, "create_workspace", map[string]any{
		"terraform_org_name": tfeOrgName,
		"workspace_name":     wsName,
		"description":        "Created by terraform-mcp-server integration tests",
	})
	require.False(t, createResult.IsError, "create_workspace should not return an error")
	require.NotEmpty(t, createResultText, "create_workspace result should not be empty")

	assert.Equal(t, wsName, gjson.Get(createResultText, "data.attributes.workspace.name").String(), "Created workspace name should match the requested name")

	wsID := gjson.Get(createResultText, "data.attributes.workspace_id").String()
	require.NotEmpty(t, wsID, "create_workspace should return a workspace_id")

	// Ensure the workspace is deleted at the end of the test using the TFE client
	// directly — independent of the tools under test.
	defer client.Workspaces.SafeDeleteByID(t.Context(), wsID)

	t.Run("Get workspace details", func(t *testing.T) {
		getResult, getResultText := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, getResult.IsError, "get_workspace_details should not return an error")
		assert.True(t, gjson.Get(getResultText, "data.attributes.success").Bool(), "Response should indicate success")
		assert.Equal(t, wsName, gjson.Get(getResultText, "data.attributes.workspace.name").String(), "Workspace name should match")
		assert.Equal(t, wsID, gjson.Get(getResultText, "data.attributes.workspace_id").String(), "get_workspace_details should return the workspace ID")
	})

	// Workspace variables tests
	runVariablesTest(t, s, wsName)

	// Tags create and read
	runWorkspaceTagsTest(t, s, wsName)

	// Update workspace
	t.Run("Update workspace", func(t *testing.T) {
		updatedDescription := "Updated by terraform-mcp-server integration tests"
		updateResult, updateResultText := callTool(t, s, "update_workspace", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
			"description":        updatedDescription,
		})
		require.False(t, updateResult.IsError, "update_workspace should not return an error")
		assert.Equal(t, updatedDescription, gjson.Get(updateResultText, "data.attributes.description").String(), "Updated description should be reflected in the response")

		// Get workspace details after update — confirm the description change persisted
		getResult, getResultText := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, getResult.IsError, "get_workspace_details after update should not return an error")
		assert.Equal(t, updatedDescription, gjson.Get(getResultText, "data.attributes.workspace.description").String(), "get_workspace_details should reflect the updated description")
	})

	// Delete workspace
	t.Run("Delete workspace", func(t *testing.T) {
		deleteResult, _ := callTool(t, s, "delete_workspace_safely", map[string]any{
			"workspace_id": wsID,
		})
		require.False(t, deleteResult.IsError, "delete_workspace_safely should not return an error")

		// Get workspace details after delete — confirm it no longer exists
		getResult, _ := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		assert.True(t, getResult.IsError, "get_workspace_details should return an error after deletion")
	})
}

func runVariablesTest(t *testing.T, s *mcp.ClientSession, wsName string) {
	t.Helper()
	t.Run("Workspace variables", func(t *testing.T) {
		// Create variable
		createVarResult, _ := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
			"key":                "test_key",
			"value":              "initial_value",
			"category":           "terraform",
			"description":        "Created by integration test",
		})
		require.False(t, createVarResult.IsError, "create_workspace_variable should not return an error")

		// List variables — confirm the variable exists and capture its ID
		listVarsResult, listVarsResultText := callTool(t, s, "list_workspace_variables", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})

		require.False(t, listVarsResult.IsError, "list_workspace_variables should not return an error")
		require.Greater(t, int(gjson.Get(listVarsResultText, "data.#").Int()), 0, "Variable list should not be empty after creation")

		varID := gjson.Get(listVarsResultText, "data.0.id").String()
		require.NotEmpty(t, varID, "Variable should have an ID")
		assert.Equal(t, "test_key", gjson.Get(listVarsResultText, "data.0.attributes.key").String(), "Variable key should match")
		assert.Equal(t, "initial_value", gjson.Get(listVarsResultText, "data.0.attributes.value").String(), "Variable value should match initial value")

		// Update variable
		updateVarResult, _ := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
			"variable_id":        varID,
			"key":                "test_key",
			"value":              "updated_value",
		})
		require.False(t, updateVarResult.IsError, "update_workspace_variable should not return an error")

		// List again — confirm the updated value
		listAfterResult, listAfterResultText := callTool(t, s, "list_workspace_variables", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, listAfterResult.IsError, "list_workspace_variables after update should not return an error")
		assert.Equal(t, "updated_value", gjson.Get(listAfterResultText, "data.0.attributes.value").String(), "Variable value should reflect the update")
	})
}

func runWorkspaceTagsTest(t *testing.T, s *mcp.ClientSession, wsName string) {
	t.Helper()
	t.Run("Workspace tags", func(t *testing.T) {
		// Create tags — one plain tag and one key:value tag binding
		createTagsResult, createTagsResultText := callTool(t, s, "create_workspace_tags", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
			"tags":               "test-tag, env:staging",
		})
		require.False(t, createTagsResult.IsError, "create_workspace_tags should not return an error")
		assert.Contains(t, createTagsResultText, wsName, "create_workspace_tags response should reference the workspace name")

		// Read tags — confirm both the plain tag and the key:value binding appear
		readTagsResult, readTagsResultText := callTool(t, s, "read_workspace_tags", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, readTagsResult.IsError, "read_workspace_tags should not return an error")
		assert.Contains(t, readTagsResultText, wsName, "read_workspace_tags response should reference the workspace name")
		assert.Contains(t, readTagsResultText, "test-tag", "read_workspace_tags response should include the plain tag")
		assert.Contains(t, readTagsResultText, "env:staging", "read_workspace_tags response should include the key:value tag binding")
	})
}

// TestWorkspaceErrorPaths exercises error branches that fires when a caller
// provides a non-existent org/workspace name or a stale workspace ID.

func TestWorkspaceErrorPaths(t *testing.T) {
	requireTfOperations(t)
	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	nonExistentOrg := randomName("org-")
	nonExistentWs := randomName("workspace-")
	const nonExistentWsID = "ws-0000000000dead"
	const nonExistentVarID = "var-0000000000dead"

	t.Run("create_workspace duplicate name", func(t *testing.T) {
		// Create the workspace once.
		wsName := randomName("workspace-")
		first, firstText := callTool(t, s, "create_workspace", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, first.IsError, "first create_workspace should succeed")

		// Register cleanup using the workspace ID returned directly by create_workspace.
		wsID := gjson.Get(firstText, "data.attributes.workspace_id").String()
		require.NotEmpty(t, wsID, "workspace should appear in list after first create")
		defer client.Workspaces.SafeDeleteByID(t.Context(), wsID)

		// Attempt to create the same workspace again — must fail.
		second, _ := callTool(t, s, "create_workspace", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		assert.True(t, second.IsError, "second create_workspace with the same name should return an error")
	})

	t.Run("list_workspaces non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "list_workspaces", map[string]any{
			"terraform_org_name": nonExistentOrg,
		})
		assert.True(t, result.IsError, "list_workspaces with a non-existent org should return an error")
	})

	t.Run("get_workspace_details non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     nonExistentWs,
		})
		assert.True(t, result.IsError, "get_workspace_details with a non-existent workspace should return an error")
	})

	t.Run("get_workspace_details non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_name":     nonExistentWs,
		})
		assert.True(t, result.IsError, "get_workspace_details with a non-existent org should return an error")
	})

	t.Run("update_workspace non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "update_workspace", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     nonExistentWs,
			"description":        "should never land",
		})
		assert.True(t, result.IsError, "update_workspace with a non-existent workspace should return an error")
	})

	// update_workspace — non-existent org name
	t.Run("update_workspace non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "update_workspace", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_name":     nonExistentWs,
			"description":        "should never land",
		})
		assert.True(t, result.IsError, "update_workspace with a non-existent org should return an error")
	})

	// delete_workspace_safely — non-existent workspace ID
	t.Run("delete_workspace_safely non-existent workspace ID", func(t *testing.T) {
		result, _ := callTool(t, s, "delete_workspace_safely", map[string]any{
			"workspace_id": nonExistentWsID,
		})
		assert.True(t, result.IsError, "delete_workspace_safely with a non-existent workspace ID should return an error")
	})

	// create_workspace_variable — non-existent workspace
	t.Run("create_workspace_variable non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     nonExistentWs,
			"key":                "some_key",
			"value":              "some_value",
		})
		assert.True(t, result.IsError, "create_workspace_variable with a non-existent workspace should return an error")
	})

	// create_workspace_variable — non-existent org
	t.Run("create_workspace_variable non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_name":     nonExistentWs,
			"key":                "some_key",
			"value":              "some_value",
		})
		assert.True(t, result.IsError, "create_workspace_variable with a non-existent org should return an error")
	})

	// update_workspace_variable — non-existent workspace
	t.Run("update_workspace_variable non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     nonExistentWs,
			"variable_id":        nonExistentVarID,
			"key":                "some_key",
			"value":              "some_value",
		})
		assert.True(t, result.IsError, "update_workspace_variable with a non-existent workspace should return an error")
	})

	// update_workspace_variable — non-existent variable ID (workspace exists)
	t.Run("update_workspace_variable non-existent variable ID", func(t *testing.T) {
		// Create a throw-away workspace so the workspace lookup succeeds, but the
		// variable ID does not exist.
		wsName := randomName("workspace-")
		createResult, createText := callTool(t, s, "create_workspace", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, createResult.IsError, "setup create_workspace should succeed")

		wsID := gjson.Get(createText, "data.attributes.workspace_id").String()
		require.NotEmpty(t, wsID)
		defer client.Workspaces.SafeDeleteByID(t.Context(), wsID)

		result, _ := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
			"variable_id":        nonExistentVarID,
			"key":                "some_key",
			"value":              "some_value",
		})
		assert.True(t, result.IsError, "update_workspace_variable with a non-existent variable ID should return an error")
	})
}
