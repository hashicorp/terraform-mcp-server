// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCreateNoCodeWorkspace(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)

	project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
		Name: randomName("nocode-project-"),
	})
	require.NoError(t, err, "failed to create test project")
	defer client.Projects.Delete(t.Context(), project.ID)

	module, err := client.RegistryModules.Create(t.Context(), tfeOrgName, tfe.RegistryModuleCreateOptions{
		Name:         tfe.String(randomName("nocode-module-")),
		Provider:     tfe.String("testprovider"),
		RegistryName: tfe.PrivateRegistry,
	})
	require.NoError(t, err, "failed to create test private module")

	moduleID := tfe.RegistryModuleID{
		Organization: tfeOrgName,
		Namespace:    module.Namespace,
		Name:         module.Name,
		Provider:     module.Provider,
		RegistryName: tfe.PrivateRegistry,
	}
	defer client.RegistryModules.DeleteProvider(t.Context(), moduleID)

	const moduleVersion = "1.0.0"
	version, err := client.RegistryModules.CreateVersion(t.Context(), moduleID, tfe.RegistryModuleCreateVersionOptions{
		Version: tfe.String(moduleVersion),
	})
	require.NoError(t, err, "failed to create test private module version")
	require.NoError(t, client.RegistryModules.Upload(t.Context(), *version, "testdata/no_code_workspace_module"), "failed to upload test private module")

	waitFor(t, 2*time.Minute, fmt.Sprintf("private module %q version %s to finish processing", module.Name, moduleVersion), func(ctx context.Context) (*tfe.TerraformRegistryModule, error) {
		registryModule, err := client.RegistryModules.ReadTerraformRegistryModule(ctx, moduleID, moduleVersion)
		if err != nil {
			return nil, err
		}
		if registryModule == nil || len(registryModule.Root.Inputs) != 2 || len(registryModule.Root.Outputs) != 2 {
			return nil, nil
		}
		return registryModule, nil
	})

	noCodeModule, err := client.RegistryNoCodeModules.Create(t.Context(), tfeOrgName, tfe.RegistryNoCodeModuleCreateOptions{
		RegistryModule: module,
		Enabled:        tfe.Bool(true),
		VersionPin:     moduleVersion,
	})
	require.NoError(t, err, "failed to enable the test module for no-code provisioning")
	defer client.RegistryNoCodeModules.Delete(t.Context(), noCodeModule.ID)

	workspaceName := randomName("nocode_workspace_")
	// No-code workspace creation starts a run. Force-delete this test-owned
	// workspace so a pending manual-apply run cannot block cleanup.
	defer client.Workspaces.Delete(t.Context(), tfeOrgName, workspaceName)

	result, resultText := callTool(t, s, "create_no_code_workspace", map[string]any{
		"no_code_module_id": noCodeModule.ID,
		"workspace_name":    workspaceName,
		"project_id":        project.ID,
		"auto_apply":        false,
	})
	require.False(t, result.IsError, "create_no_code_workspace should not return an error: %s", resultText)
	require.NotEmpty(t, resultText, "create_no_code_workspace result should not be empty")

	workspaceID := gjson.Get(resultText, pick("data.attributes.workspace_id", "workspace_id")).String()
	require.NotEmpty(t, workspaceID, "create_no_code_workspace should return a workspace_id")

	workspace, err := client.Workspaces.ReadByID(t.Context(), workspaceID)
	require.NoError(t, err, "created no-code workspace could not be read through the TFE API")
	assert.Equal(t, workspaceName, workspace.Name)
	assert.False(t, workspace.AutoApply)
	assert.Equal(t, project.ID, workspace.Project.ID)

	variables, err := client.Variables.List(t.Context(), workspaceID, nil)
	require.NoError(t, err, "failed to list variables for the created no-code workspace")
	require.Len(t, variables.Items, 1, "only the required module input should be set on the workspace")
	assert.Equal(t, "name", variables.Items[0].Key)
	assert.Equal(t, "integration-test", variables.Items[0].Value)
}

func TestWorkspaceHappyPath(t *testing.T) {
	// TODO: Switch to the official SDK requests and assertions, then remove the
	// legacy references once the SDK migration is complete.
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

	assert.Equal(t, wsName, gjson.Get(createResultText, pick("data.attributes.workspace.name", "workspace_name")).String(), "Created workspace name should match the requested name")

	wsID := gjson.Get(createResultText, pick("data.attributes.workspace_id", "workspace_id")).String()
	require.NotEmpty(t, wsID, "create_workspace should return a workspace_id")

	// Ensure the workspace is deleted at the end of the test using the TFE client
	// directly — independent of the tools under test.
	defer client.Workspaces.SafeDeleteByID(t.Context(), wsID)

	t.Run("list workspaces in org", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_workspaces", map[string]any{
			"terraform_org_name": tfeOrgName,
		})
		require.False(t, result.IsError, "list_workspaces should not return an error")
		require.NotEmpty(t, resultText, "list_workspaces should return a non-empty response")

		assert.Greater(t, int(gjson.Get(resultText, "items.#").Int()), 0, "list_workspaces should return at least one workspace")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.id").String(), "workspace items should contain an id")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.workspace_name").String(), "workspace items should contain a workspace_name")

		// Confirm the workspace created for this test appears in the list.
		found := false
		gjson.Get(resultText, "items").ForEach(func(_, item gjson.Result) bool {
			if item.Get("id").String() == wsID {
				found = true
				return false
			}
			return true
		})
		assert.True(t, found, "list_workspaces should include the workspace created for this test (id: %s)", wsID)
	})

	t.Run("Get workspace details", func(t *testing.T) {
		getResult, getResultText := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, getResult.IsError, "get_workspace_details should not return an error")
		if !officialEndpoint() {
			assert.True(t, gjson.Get(getResultText, "data.attributes.success").Bool(), "Response should indicate success")
		}
		assert.Equal(t, wsName, gjson.Get(getResultText, pick("data.attributes.workspace.name", "workspace_name")).String(), "Workspace name should match")
		assert.Equal(t, wsID, gjson.Get(getResultText, pick("data.attributes.workspace_id", "workspace_id")).String(), "get_workspace_details should return the workspace ID")
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
		assert.Equal(t, updatedDescription, gjson.Get(updateResultText, pick("data.attributes.description", "description")).String(), "Updated description should be reflected in the response")

		// Get workspace details after update — confirm the description change persisted
		getResult, getResultText := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, getResult.IsError, "get_workspace_details after update should not return an error")
		assert.Equal(t, updatedDescription, gjson.Get(getResultText, pick("data.attributes.workspace.description", "description")).String(), "get_workspace_details should reflect the updated description")
	})

	// Delete workspace
	t.Run("Delete workspace", func(t *testing.T) {
		deleteResult, deleteResultText := callTool(t, s, "delete_workspace_safely", map[string]any{
			"workspace_id": wsID,
		})
		require.False(t, deleteResult.IsError, "delete_workspace_safely should not return an error")
		if officialEndpoint() {
			assert.Equal(t, wsID, gjson.Get(deleteResultText, "workspace_id").String(), "delete_workspace_safely should return the workspace ID")
			assert.Equal(t, wsName, gjson.Get(deleteResultText, "workspace_name").String(), "delete_workspace_safely should return the workspace name")
			assert.True(t, gjson.Get(deleteResultText, "deleted").Bool(), "delete_workspace_safely should report the workspace as deleted")
		}

		// Get workspace details after delete — confirm it no longer exists
		getResult, _ := callTool(t, s, "get_workspace_details", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		assert.True(t, getResult.IsError, "get_workspace_details should return an error after deletion")
	})
}

func TestForceUnlockWorkspace(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	workspaceName := randomName("unlock-test-")
	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{Name: &workspaceName})
	require.NoError(t, err, "failed to create test workspace")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	lockReason := "Test force_unlock_workspace integration"
	workspace, err = client.Workspaces.Lock(t.Context(), workspace.ID, tfe.WorkspaceLockOptions{Reason: &lockReason})
	require.NoError(t, err, "failed to lock test workspace")
	// Ensure the workspace is unlocked at the end of the test in case the force unlock tool fails, so that the workspace can be deleted.
	defer client.Workspaces.ForceUnlock(t.Context(), workspace.ID)
	require.True(t, workspace.Locked, "setup should leave the workspace locked")

	result, resultText := callTool(t, s, "force_unlock_workspace", map[string]any{
		"workspace_id": workspace.ID,
	})
	require.False(t, result.IsError, "force_unlock_workspace should not return an error")
	assert.Contains(t, resultText, workspace.ID, "response should reference the unlocked workspace")

	workspace, err = client.Workspaces.ReadByID(t.Context(), workspace.ID)
	require.NoError(t, err, "failed to read workspace after force unlock")
	assert.False(t, workspace.Locked, "workspace should be unlocked in the TFE API")
}

func runVariablesTest(t *testing.T, s *mcp.ClientSession, wsName string) {
	t.Helper()
	t.Run("Workspace variables", func(t *testing.T) {
		// Create variable
		createVarResult, createVarResultText := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name":            tfeOrgName,
			"workspace_name":                wsName,
			pick("key", "variable_key"):     "test_key",
			pick("value", "variable_value"): "initial_value",
			"category":                      "terraform",
			"description":                   "Created by integration test",
		})
		require.False(t, createVarResult.IsError, "create_workspace_variable should not return an error")
		if officialEndpoint() {
			assert.Equal(t, "test_key", gjson.Get(createVarResultText, "key").String(), "created variable key should match")
			assert.Equal(t, "initial_value", gjson.Get(createVarResultText, "value").String(), "created variable value should match")
			assert.Equal(t, "terraform", gjson.Get(createVarResultText, "category").String(), "created variable category should match")
		}

		// List variables — confirm the variable exists and capture its ID
		listVarsResult, listVarsResultText := callTool(t, s, "list_workspace_variables", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})

		require.False(t, listVarsResult.IsError, "list_workspace_variables should not return an error")
		require.Greater(t, int(gjson.Get(listVarsResultText, listPath()+".#").Int()), 0, "variable list should not be empty after creation")

		varID := gjson.Get(listVarsResultText, pick("data.0.id", "items.0.id")).String()
		require.NotEmpty(t, varID, "Variable should have an ID")
		assert.Equal(t, "test_key", gjson.Get(listVarsResultText, pick("data.0.attributes.key", "items.0.key")).String(), "variable key should match")
		assert.Equal(t, "initial_value", gjson.Get(listVarsResultText, pick("data.0.attributes.value", "items.0.value")).String(), "variable value should match initial value")

		// Update variable
		updateVarResult, updateVarResultText := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name":            tfeOrgName,
			"workspace_name":                wsName,
			"variable_id":                   varID,
			pick("key", "variable_key"):     "test_key",
			pick("value", "variable_value"): "updated_value",
		})
		require.False(t, updateVarResult.IsError, "update_workspace_variable should not return an error")
		if officialEndpoint() {
			assert.Equal(t, "test_key", gjson.Get(updateVarResultText, "key").String(), "updated variable key should match")
			assert.Equal(t, "updated_value", gjson.Get(updateVarResultText, "value").String(), "updated variable value should match")
		}

		// List again — confirm the updated value
		listAfterResult, listAfterResultText := callTool(t, s, "list_workspace_variables", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		require.False(t, listAfterResult.IsError, "list_workspace_variables after update should not return an error")
		assert.Equal(t, "updated_value", gjson.Get(listAfterResultText, pick("data.0.attributes.value", "items.0.value")).String(), "variable value should reflect the update")
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
		wsID := gjson.Get(firstText, pick("data.attributes.workspace_id", "workspace_id")).String()
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
		result, resultText := callTool(t, s, "list_workspaces", map[string]any{
			"terraform_org_name": nonExistentOrg,
		})
		assert.True(t, result.IsError, "list_workspaces with a non-existent org should return an error")
		assert.Contains(t, resultText, nonExistentOrg, "error should reference the org that was not found")
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
		result, resultText := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name":            tfeOrgName,
			"workspace_name":                nonExistentWs,
			pick("key", "variable_key"):     "some_key",
			pick("value", "variable_value"): "some_value",
			"category":                      "terraform",
		})
		assert.True(t, result.IsError, "create_workspace_variable with a non-existent workspace should return an error")
		assert.NotContains(t, resultText, "missing properties", "should fail on the workspace lookup, not schema validation")
	})

	// create_workspace_variable — non-existent org
	t.Run("create_workspace_variable non-existent org", func(t *testing.T) {
		result, resultText := callTool(t, s, "create_workspace_variable", map[string]any{
			"terraform_org_name":            nonExistentOrg,
			"workspace_name":                nonExistentWs,
			pick("key", "variable_key"):     "some_key",
			pick("value", "variable_value"): "some_value",
			"category":                      "terraform",
		})
		assert.True(t, result.IsError, "create_workspace_variable with a non-existent org should return an error")
		assert.NotContains(t, resultText, "missing properties", "should fail on the org lookup, not schema validation")
	})

	// update_workspace_variable — non-existent workspace
	t.Run("update_workspace_variable non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name":            tfeOrgName,
			"workspace_name":                nonExistentWs,
			"variable_id":                   nonExistentVarID,
			pick("key", "variable_key"):     "some_key",
			pick("value", "variable_value"): "some_value",
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

		wsID := gjson.Get(createText, pick("data.attributes.workspace_id", "workspace_id")).String()
		require.NotEmpty(t, wsID)
		defer client.Workspaces.SafeDeleteByID(t.Context(), wsID)

		result, _ := callTool(t, s, "update_workspace_variable", map[string]any{
			"terraform_org_name":            tfeOrgName,
			"workspace_name":                wsName,
			"variable_id":                   nonExistentVarID,
			pick("key", "variable_key"):     "some_key",
			pick("value", "variable_value"): "some_value",
		})
		assert.True(t, result.IsError, "update_workspace_variable with a non-existent variable ID should return an error")
	})
}
