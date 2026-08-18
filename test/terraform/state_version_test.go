// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	_ "embed"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

//go:embed testdata/state_version_test.tf
var stateVersionTestConfiguration string

func TestListStateVersionsHappyPath(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	// Create an isolated remote workspace for the run that will produce a state version.
	client := tfeClient(t)
	workspaceName := randomName("sv-test-")
	executionMode := "remote"

	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name:          &workspaceName,
		ExecutionMode: &executionMode,
		AutoApply:     tfe.Bool(false),
	})
	require.NoError(t, err, "failed to create test workspace")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	// Upload a Terraform configuration then create and apply a run via the TFE
	// client directly — we are only testing list_state_versions here.
	uploadStateVersionTestConfiguration(t, client, workspace.ID)

	runMessage := "Created by terraform-mcp-server list state version integration tests"
	run, err := client.Runs.Create(t.Context(), tfe.RunCreateOptions{
		Workspace: workspace,
		AutoApply: tfe.Bool(false),
		Message:   &runMessage,
	})
	require.NoError(t, err, "failed to create run")

	waitForRun(t, client, run.ID, "become confirmable", func(r *tfe.Run) bool {
		return r.Actions != nil && r.Actions.IsConfirmable
	})

	applyComment := "Approved by state version integration tests"
	require.NoError(t, client.Runs.Apply(t.Context(), run.ID, tfe.RunApplyOptions{Comment: &applyComment}), "failed to apply run")

	// A state version is only created after a successful apply.
	waitForRun(t, client, run.ID, "finish applying", func(r *tfe.Run) bool {
		return r.Status == tfe.RunApplied
	})

	t.Run("list_state_versions returns at least one state version", func(t *testing.T) {
		svResult, svText := callTool(t, s, "list_state_versions", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     workspaceName,
		})
		require.False(t, svResult.IsError, "list_state_versions should not return an error")
		require.NotEmpty(t, svText, "list_state_versions response must not be empty")

		require.Greater(t, int(gjson.Get(svText, "items.#").Int()), 0, "list_state_versions should return at least one item")

		toolSVID := gjson.Get(svText, "items.0.id").String()
		require.NotEmpty(t, toolSVID, "state version item should have an id")

		// Cross-verify against the TFE API directly.
		svList, err := client.StateVersions.List(t.Context(), &tfe.StateVersionListOptions{
			Organization: tfeOrgName,
			Workspace:    workspaceName,
		})
		require.NoError(t, err, "TFE API should be able to list state versions")
		require.NotEmpty(t, svList.Items, "TFE API should return at least one state version")
		assert.Equal(t, svList.Items[0].ID, toolSVID, "tool state version ID should match the TFE API")

		// Verify the summary fields are populated.
		assert.NotEmpty(t, gjson.Get(svText, "items.0.created_at").String(), "state version should have a created_at timestamp")
		assert.NotEmpty(t, gjson.Get(svText, "items.0.terraform_version").String(), "state version should include the terraform_version")
		assert.True(t, gjson.Get(svText, "items.0.serial").Exists(), "state version should include a serial number")
	})
}

func TestListStateVersionsErrorPaths(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)

	nonExistentOrg := "org-doesnotexist123"
	nonExistentWs := "ws-doesnotexist123"

	t.Run("list_state_versions non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "list_state_versions", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_name":     nonExistentWs,
		})
		assert.True(t, result.IsError, "list_state_versions with a non-existent org should return an error")
	})

	t.Run("list_state_versions non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "list_state_versions", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     nonExistentWs,
		})
		assert.True(t, result.IsError, "list_state_versions with a non-existent workspace should return an error")
	})

	t.Run("list_state_versions empty workspace has no state versions", func(t *testing.T) {
		// Create a real workspace that has never had a run — it will have no state versions.
		wsName := randomName("sv-empty-")
		workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
			Name: &wsName,
		})
		require.NoError(t, err, "failed to create empty test workspace")
		defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

		result, _ := callTool(t, s, "list_state_versions", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     wsName,
		})
		assert.True(t, result.IsError, "list_state_versions on a workspace with no state should return an error")
	})
}

func TestGetStateVersionHappyPath(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	workspaceName := randomName("sv-get-")
	executionMode := "remote"
	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name:          &workspaceName,
		ExecutionMode: &executionMode,
		AutoApply:     tfe.Bool(false),
	})
	require.NoError(t, err, "failed to create test workspace")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	// Upload a Terraform configuration then create and apply a run via the TFE
	// client directly — we are only testing list_state_versions here.
	uploadStateVersionTestConfiguration(t, client, workspace.ID)

	runMessage := "Created by terraform-mcp-server get state version integration tests"
	run, err := client.Runs.Create(t.Context(), tfe.RunCreateOptions{
		Workspace: workspace,
		AutoApply: tfe.Bool(false),
		Message:   &runMessage,
	})
	require.NoError(t, err, "failed to create run")

	waitForRun(t, client, run.ID, "become confirmable", func(r *tfe.Run) bool {
		return r.Actions != nil && r.Actions.IsConfirmable
	})
	applyComment := "Approved by get state version integration tests"
	require.NoError(t, client.Runs.Apply(t.Context(), run.ID, tfe.RunApplyOptions{Comment: &applyComment}), "failed to apply run")
	waitForRun(t, client, run.ID, "finish applying", func(r *tfe.Run) bool {
		return r.Status == tfe.RunApplied
	})

	stateVersions, err := client.StateVersions.List(t.Context(), &tfe.StateVersionListOptions{
		Organization: tfeOrgName,
		Workspace:    workspaceName,
	})
	require.NoError(t, err, "failed to list created state versions")
	require.NotEmpty(t, stateVersions.Items, "applied run should create a state version")
	stateVersion := stateVersions.Items[0]

	// Download the raw state file once and reuse it across sub-tests so we only
	// hit the download URL a single time.
	stateBytes, err := client.StateVersions.Download(t.Context(), stateVersion.DownloadURL)
	require.NoError(t, err, "failed to download state file")
	stateJSON := string(stateBytes)

	t.Run("gets an exact state version by state_version_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_state_version", map[string]any{
			"state_version_id": stateVersion.ID,
		})

		require.False(t, result.IsError, "get_state_version should not return an error")
		require.NotEmpty(t, resultText, "get_state_version response must not be empty")
		assert.Equal(t, stateVersion.ID, gjson.Get(resultText, "data.id").String())
		assert.Equal(t, stateVersion.Serial, gjson.Get(resultText, "data.attributes.serial").Int())
		assert.NotEmpty(t, gjson.Get(resultText, "data.attributes.created-at").String())

		// Verify the state file contains the resources from the test fixture.
		assert.Contains(t, stateJSON, `"name": "resource_one"`, "state file should contain resource_one from the test fixture")
		assert.Contains(t, stateJSON, `"name": "resource_two"`, "state file should contain resource_two from the test fixture")
	})

	t.Run("gets the current state version by workspace_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_state_version", map[string]any{
			"workspace_id": workspace.ID,
		})

		require.False(t, result.IsError, "get_state_version should not return an error")
		require.NotEmpty(t, resultText, "get_state_version response must not be empty")
		assert.Equal(t, stateVersion.ID, gjson.Get(resultText, "data.id").String())

		// Verify the state file contains the resources from the test fixture.
		assert.Contains(t, stateJSON, `"name": "resource_one"`, "state file should contain resource_one from the test fixture")
		assert.Contains(t, stateJSON, `"name": "resource_two"`, "state file should contain resource_two from the test fixture")
	})
}

func TestGetStateVersionErrorPaths(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	nonExistentSV := "sv-doesnotexist123"
	nonExistentWs := "ws-doesnotexist123"

	t.Run("requires a state version or workspace ID", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_state_version", map[string]any{})
		require.True(t, result.IsError, "get_state_version without an identifier should return an error")
		assert.Contains(t, resultText, "One of state_version_id or workspace_id must be provided")
	})

	t.Run("rejects a non-existent state version", func(t *testing.T) {
		result, _ := callTool(t, s, "get_state_version", map[string]any{
			"state_version_id": nonExistentSV,
		})
		require.True(t, result.IsError, "get_state_version should return an error for an unknown state version")
	})

	t.Run("rejects a non-existent workspace", func(t *testing.T) {
		result, _ := callTool(t, s, "get_state_version", map[string]any{
			"workspace_id": nonExistentWs,
		})
		require.True(t, result.IsError, "get_state_version should return an error for an unknown workspace")
	})
}

func uploadStateVersionTestConfiguration(t *testing.T, client *tfe.Client, workspaceID string) {
	uploadConfiguration(t, client, workspaceID, stateVersionTestConfiguration)
}
