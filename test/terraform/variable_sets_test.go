// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestVariableSetHappyPath exercises the full lifecycle of a variable set:
// create → list → add a variable → delete the variable → attach to a workspace
// → detach from a workspace. A real workspace is created via the TFE client so
// that the attach/detach tools have something concrete to target.
func TestVariableSetHappyPath(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	// ── Setup: create a workspace via the TFE client ──────────────────────────
	wsName := randomName("varset-ws-")
	ws, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name: tfe.String(wsName),
	})
	require.NoError(t, err, "setup: failed to create test workspace via TFE API")
	defer client.Workspaces.SafeDeleteByID(t.Context(), ws.ID)

	// ── create_variable_set ───────────────────────────────────────────────────
	varSetName := randomName("varsset-")
	createResult, createResultText := callTool(t, s, "create_variable_set", map[string]any{
		"terraform_org_name": tfeOrgName,
		"name":               varSetName,
		"description":        "Created by terraform-mcp-server integration tests",
		"global":             false,
	})
	require.False(t, createResult.IsError, "create_variable_set should not return an error")
	require.NotEmpty(t, createResultText, "create_variable_set result should not be empty")
	assert.Contains(t, createResultText, varSetName, "create_variable_set response should reference the variable set name")

	// Extract the variable set ID from the response text ("…with ID varset-xxxx").
	// The response format is: "Successfully created variable set <name> with ID <id>"
	varSetID := extractID(createResultText, "varset-")
	require.NotEmpty(t, varSetID, "create_variable_set response must contain a varset- ID")

	// Ensure the variable set is deleted at the end of the test regardless of
	// what the tools under test do.
	defer client.VariableSets.Delete(t.Context(), varSetID)

	// Verify creation via the TFE API directly.
	varSet, err := client.VariableSets.Read(t.Context(), varSetID, nil)
	require.NoError(t, err, "variable set was reported as created but could not be read via the TFE API")
	assert.Equal(t, varSetName, varSet.Name, "variable set name in TFE API should match the requested name")
	assert.Equal(t, "Created by terraform-mcp-server integration tests", varSet.Description, "variable set description in TFE API should match")

	t.Run("list_variable_sets returns the created variable set", func(t *testing.T) {
		listResult, listResultText := callTool(t, s, "list_variable_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"query":              varSetName,
		})
		require.False(t, listResult.IsError, "list_variable_sets should not return an error")
		require.NotEmpty(t, listResultText, "list_variable_sets result should not be empty")

		assert.Greater(t, int(gjson.Get(listResultText, "data.#").Int()), 0,
			"list_variable_sets should return at least one item after creation")
		assert.Contains(t, listResultText, varSetID,
			"list_variable_sets response should include the created variable set ID")
	})

	var variableID string
	t.Run("create_variable_in_variable_set adds a variable", func(t *testing.T) {
		createVarResult, createVarResultText := callTool(t, s, "create_variable_in_variable_set", map[string]any{
			"variable_set_id": varSetID,
			"key":             "test_key",
			"value":           "test_value",
			"category":        "terraform",
			"description":     "Created by integration test",
		})
		require.False(t, createVarResult.IsError, "create_variable_in_variable_set should not return an error")
		require.NotEmpty(t, createVarResultText, "create_variable_in_variable_set result should not be empty")
		assert.Contains(t, createVarResultText, "test_key",
			"create_variable_in_variable_set response should reference the variable key")
		assert.Contains(t, createVarResultText, varSetID,
			"create_variable_in_variable_set response should reference the variable set ID")

		// Extract the variable ID from the response (format: "…variable <key> with ID var-xxxx in variable set…").
		variableID = extractID(createVarResultText, "var-")
		require.NotEmpty(t, variableID, "create_variable_in_variable_set response must contain a var- ID")

		// Verify via the TFE API directly.
		variables, err := client.VariableSetVariables.List(t.Context(), varSetID, nil)
		require.NoError(t, err, "failed to list variables in variable set via TFE API")
		require.NotEmpty(t, variables.Items, "variable set should have at least one variable after creation")
		assert.Equal(t, "test_key", variables.Items[0].Key, "variable key in TFE API should match")
	})

	t.Run("delete_variable_in_variable_set removes the variable", func(t *testing.T) {
		require.NotEmpty(t, variableID, "prerequisite: variable ID must be set from the create step")

		deleteVarResult, deleteVarResultText := callTool(t, s, "delete_variable_in_variable_set", map[string]any{
			"variable_set_id": varSetID,
			"variable_id":     variableID,
		})
		require.False(t, deleteVarResult.IsError, "delete_variable_in_variable_set should not return an error")
		assert.Contains(t, deleteVarResultText, variableID,
			"delete_variable_in_variable_set response should reference the deleted variable ID")

		// Verify the variable is gone via the TFE API directly.
		variables, err := client.VariableSetVariables.List(t.Context(), varSetID, nil)
		require.NoError(t, err, "failed to list variables in variable set via TFE API after deletion")
		assert.Empty(t, variables.Items, "variable set should have no variables after deletion")
	})

	t.Run("attach_variable_set_to_workspaces attaches to the workspace", func(t *testing.T) {
		attachResult, attachResultText := callTool(t, s, "attach_variable_set_to_workspaces", map[string]any{
			"variable_set_id": varSetID,
			"workspace_ids":   ws.ID,
		})
		require.False(t, attachResult.IsError, "attach_variable_set_to_workspaces should not return an error")
		assert.Contains(t, attachResultText, varSetID,
			"attach_variable_set_to_workspaces response should reference the variable set ID")

		// Verify via the TFE API: read the variable set and confirm the workspace appears.
		includeWorkspaces := []tfe.VariableSetIncludeOpt{tfe.VariableSetWorkspaces}
		attached, err := client.VariableSets.Read(t.Context(), varSetID, &tfe.VariableSetReadOptions{
			Include: &includeWorkspaces,
		})
		require.NoError(t, err, "failed to read variable set via TFE API after attach")
		wsIDs := make([]string, 0, len(attached.Workspaces))
		for _, w := range attached.Workspaces {
			wsIDs = append(wsIDs, w.ID)
		}
		assert.Contains(t, wsIDs, ws.ID,
			"workspace should appear in the variable set's workspace list after attach")
	})

	t.Run("detach_variable_set_from_workspaces detaches from the workspace", func(t *testing.T) {
		detachResult, detachResultText := callTool(t, s, "detach_variable_set_from_workspaces", map[string]any{
			"variable_set_id": varSetID,
			"workspace_ids":   ws.ID,
		})
		require.False(t, detachResult.IsError, "detach_variable_set_from_workspaces should not return an error")
		assert.Contains(t, detachResultText, varSetID,
			"detach_variable_set_from_workspaces response should reference the variable set ID")

		// Verify via the TFE API: workspace should no longer be in the variable set.
		includeWorkspaces := []tfe.VariableSetIncludeOpt{tfe.VariableSetWorkspaces}
		detached, err := client.VariableSets.Read(t.Context(), varSetID, &tfe.VariableSetReadOptions{
			Include: &includeWorkspaces,
		})
		require.NoError(t, err, "failed to read variable set via TFE API after detach")
		wsIDs := make([]string, 0, len(detached.Workspaces))
		for _, w := range detached.Workspaces {
			wsIDs = append(wsIDs, w.ID)
		}
		assert.NotContains(t, wsIDs, ws.ID,
			"workspace should not appear in the variable set's workspace list after detach")
	})
}

// TestVariableSetErrorPaths exercises error branches for the variable set tools.
func TestVariableSetErrorPaths(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	const nonExistentVarSetID = "varset-0000000000dead"
	const nonExistentVarID = "var-0000000000dead"
	const nonExistentWsID = "ws-0000000000dead"

	t.Run("list_variable_sets non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "list_variable_sets", map[string]any{
			"terraform_org_name": randomName("org-"),
		})
		assert.True(t, result.IsError, "list_variable_sets with a non-existent org should return an error")
	})

	t.Run("create_variable_set non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "create_variable_set", map[string]any{
			"terraform_org_name": randomName("org-"),
			"name":               randomName("varset-"),
		})
		assert.True(t, result.IsError, "create_variable_set with a non-existent org should return an error")
	})

	t.Run("create_variable_in_variable_set non-existent variable set", func(t *testing.T) {
		result, _ := callTool(t, s, "create_variable_in_variable_set", map[string]any{
			"variable_set_id": nonExistentVarSetID,
			"key":             "some_key",
			"value":           "some_value",
		})
		assert.True(t, result.IsError, "create_variable_in_variable_set with a non-existent variable set should return an error")
	})

	t.Run("delete_variable_in_variable_set non-existent variable set", func(t *testing.T) {
		result, _ := callTool(t, s, "delete_variable_in_variable_set", map[string]any{
			"variable_set_id": nonExistentVarSetID,
			"variable_id":     nonExistentVarID,
		})
		assert.True(t, result.IsError, "delete_variable_in_variable_set with a non-existent variable set should return an error")
	})

	t.Run("attach_variable_set_to_workspaces non-existent variable set", func(t *testing.T) {
		// Create a real workspace so only the variable set lookup fails.
		wsName := randomName("varset-err-ws-")
		ws, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
			Name: tfe.String(wsName),
		})
		require.NoError(t, err, "setup: failed to create workspace via TFE API")
		defer client.Workspaces.SafeDeleteByID(t.Context(), ws.ID)

		result, _ := callTool(t, s, "attach_variable_set_to_workspaces", map[string]any{
			"variable_set_id": nonExistentVarSetID,
			"workspace_ids":   ws.ID,
		})
		assert.True(t, result.IsError, "attach_variable_set_to_workspaces with a non-existent variable set should return an error")
	})

	t.Run("attach_variable_set_to_workspaces non-existent workspace", func(t *testing.T) {
		// Create a real variable set so only the workspace lookup fails.
		varSetName := randomName("varset-err-")
		varSet, err := client.VariableSets.Create(t.Context(), tfeOrgName, &tfe.VariableSetCreateOptions{
			Name:   tfe.String(varSetName),
			Global: tfe.Bool(false),
		})
		require.NoError(t, err, "setup: failed to create variable set via TFE API")
		defer client.VariableSets.Delete(t.Context(), varSet.ID)

		result, _ := callTool(t, s, "attach_variable_set_to_workspaces", map[string]any{
			"variable_set_id": varSet.ID,
			"workspace_ids":   nonExistentWsID,
		})
		assert.True(t, result.IsError, "attach_variable_set_to_workspaces with a non-existent workspace should return an error")
	})

	t.Run("detach_variable_set_from_workspaces non-existent variable set", func(t *testing.T) {
		result, _ := callTool(t, s, "detach_variable_set_from_workspaces", map[string]any{
			"variable_set_id": nonExistentVarSetID,
			"workspace_ids":   nonExistentWsID,
		})
		assert.True(t, result.IsError, "detach_variable_set_from_workspaces with a non-existent variable set should return an error")
	})
}

// extractID finds the first token in s that starts with prefix and returns it,
// trimmed of any trailing whitespace. Returns "" if not found.
func extractID(s, prefix string) string {
	idx := strings.Index(s, prefix)
	if idx == -1 {
		return ""
	}
	rest := s[idx:]
	if end := strings.IndexAny(rest, " \t\n\r"); end != -1 {
		return rest[:end]
	}
	return rest
}
