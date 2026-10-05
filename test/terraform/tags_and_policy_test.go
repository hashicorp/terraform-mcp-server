package terraform

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListWorkspacePolicySets(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	requirePolicySetsEntitlement(t, client)

	s := newTestingSession(t)
	defer s.Close()

	// Create a throw-away workspace via the TFE API — independent of the tool under test.
	wsName := randomName("ws-policy-")
	ws, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name: &wsName,
	})
	require.NoError(t, err, "setup: failed to create workspace via TFE API")
	defer client.Workspaces.SafeDeleteByID(t.Context(), ws.ID)

	// Create a non-global policy set directly attached to the workspace.
	psName := randomName("ps-test-")
	ps, err := client.PolicySets.Create(t.Context(), tfeOrgName, tfe.PolicySetCreateOptions{
		Name:       tfe.String(psName),
		Workspaces: []*tfe.Workspace{{ID: ws.ID}},
	})
	require.NoError(t, err, "setup: failed to create policy set via TFE API")
	defer client.PolicySets.Delete(t.Context(), ps.ID)

	t.Run("returns directly attached policy set", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       ws.ID,
		})

		require.False(t, result.IsError, "list_workspace_policy_sets should not return an error")
		require.NotEmpty(t, resultText, "list_workspace_policy_sets should return a non-empty response")

		psResult := gjson.Get(resultText, fmt.Sprintf("#(id==%q)", ps.ID))
		require.True(t, psResult.Exists(), "response should contain the directly attached policy set")
		assert.Equal(t, psName, psResult.Get("name").String(), "response should contain the policy set name")
		assert.Equal(t, "directly attached", psResult.Get("reason").String(), "policy set should be reported as directly attached")
		assert.False(t, psResult.Get("global").Bool(), "policy set should not be global")
	})

	t.Run("returns global policy set for any workspace", func(t *testing.T) {
		// Create a global policy set — it applies to every workspace in the org,
		// regardless of whether the workspace is explicitly attached.
		globalPsName := randomName("ps-global-")
		globalPs, err := client.PolicySets.Create(t.Context(), tfeOrgName, tfe.PolicySetCreateOptions{
			Name:   tfe.String(globalPsName),
			Global: tfe.Bool(true),
		})
		require.NoError(t, err, "setup: failed to create global policy set via TFE API")
		defer client.PolicySets.Delete(t.Context(), globalPs.ID)

		// Use the workspace created in the parent test — it has no direct attachment
		// to this global policy set, yet the tool must still return it.
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       ws.ID,
		})

		require.False(t, result.IsError, "list_workspace_policy_sets should not return an error")
		require.NotEmpty(t, resultText, "list_workspace_policy_sets should return a non-empty response")

		// Find the global policy set in the result array by ID using gjson query syntax.
		globalPsResult := gjson.Get(resultText, fmt.Sprintf("#(id==%q)", globalPs.ID))
		require.True(t, globalPsResult.Exists(), "response should contain the global policy set")
		assert.Equal(t, globalPsName, globalPsResult.Get("name").String(), "global policy set name should match")
		assert.Equal(t, "global", globalPsResult.Get("reason").String(), "global policy set should have reason 'global'")
		assert.True(t, globalPsResult.Get("global").Bool(), "global policy set should have global=true")
	})

	t.Run("returns an error when the workspace is not in the given org", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": "org-doesnotexist123",
			"workspace_id":       ws.ID,
		})
		require.True(t, result.IsError, "list_workspace_policy_sets should reject a workspace/org mismatch")
		assert.Contains(t, resultText, "belongs to organization", "error should explain the org mismatch")
	})
}

func TestListWorkspacePolicySetsErrorPaths(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	t.Run("returns an error for a non-existent workspace", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       "ws-doesnotexist123",
		})
		require.True(t, result.IsError, "list_workspace_policy_sets should reject a non-existent workspace ID")
		assert.Contains(t, resultText, "workspace not found", "error should identify the workspace as not found")
	})
}

func TestAttachPolicySetToWorkspaces(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	requirePolicySetsEntitlement(t, client)

	s := newTestingSession(t)
	defer s.Close()

	// Create resources directly through the TFE API so the tool call is the
	// only operation under test.
	workspaceIDs := make([]string, 0, 2)
	for range 2 {
		wsName := randomName("ws-attach-")
		ws, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
			Name: &wsName,
		})
		require.NoError(t, err, "setup: failed to create workspace via TFE API")
		workspaceIDs = append(workspaceIDs, ws.ID)
		defer client.Workspaces.SafeDeleteByID(t.Context(), ws.ID)
	}

	psName := randomName("ps-attach-")
	ps, err := client.PolicySets.Create(t.Context(), tfeOrgName, tfe.PolicySetCreateOptions{
		Name: tfe.String(psName),
	})
	require.NoError(t, err, "setup: failed to create policy set via TFE API")
	defer client.PolicySets.Delete(t.Context(), ps.ID)

	t.Run("attaches policy set to multiple workspaces", func(t *testing.T) {
		result, resultText := callTool(t, s, "attach_policy_set_to_workspaces", map[string]any{
			"policy_set_id": ps.ID,
			"workspace_ids": strings.Join(workspaceIDs, ", "),
		})

		require.False(t, result.IsError, "attach_policy_set_to_workspaces should not return an error: %s", resultText)
		assert.Contains(t, resultText, ps.ID, "success response should reference the policy set ID")
		assert.Contains(t, resultText, "2 workspace(s)", "success response should report both workspaces")

		attachedPolicySet, err := client.PolicySets.ReadWithOptions(t.Context(), ps.ID, &tfe.PolicySetReadOptions{
			Include: []tfe.PolicySetIncludeOpt{tfe.PolicySetWorkspaces},
		})
		require.NoError(t, err, "verification: failed to read policy set after attachment")

		attachedWorkspaceIDs := make([]string, 0, len(attachedPolicySet.Workspaces))
		for _, workspace := range attachedPolicySet.Workspaces {
			attachedWorkspaceIDs = append(attachedWorkspaceIDs, workspace.ID)
		}
		assert.ElementsMatch(t, workspaceIDs, attachedWorkspaceIDs, "policy set should be attached to both requested workspaces")
	})
}

func TestAttachPolicySetToWorkspacesErrorPaths(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	t.Run("rejects empty workspace IDs", func(t *testing.T) {
		result, resultText := callTool(t, s, "attach_policy_set_to_workspaces", map[string]any{
			"policy_set_id": "polset-doesnotexist123",
			"workspace_ids": " , ",
		})

		require.True(t, result.IsError, "attach_policy_set_to_workspaces should reject empty workspace IDs")
		assert.Contains(t, resultText, "no valid workspace IDs provided")
	})

	t.Run("rejects a non-existent policy set", func(t *testing.T) {
		result, _ := callTool(t, s, "attach_policy_set_to_workspaces", map[string]any{
			"policy_set_id": "polset-doesnotexist123",
			"workspace_ids": "ws-doesnotexist123",
		})

		assert.True(t, result.IsError, "attach_policy_set_to_workspaces should reject a non-existent policy set")
	})

	t.Run("rejects a non-existent workspace", func(t *testing.T) {
		client := tfeClient(t)
		requirePolicySetsEntitlement(t, client)

		psName := randomName("ps-attach-error-")
		ps, err := client.PolicySets.Create(t.Context(), tfeOrgName, tfe.PolicySetCreateOptions{
			Name: tfe.String(psName),
		})
		require.NoError(t, err, "setup: failed to create policy set via TFE API")
		defer client.PolicySets.Delete(t.Context(), ps.ID)

		result, _ := callTool(t, s, "attach_policy_set_to_workspaces", map[string]any{
			"policy_set_id": ps.ID,
			"workspace_ids": "ws-doesnotexist123",
		})

		assert.True(t, result.IsError, "attach_policy_set_to_workspaces should reject a non-existent workspace")
	})
}
