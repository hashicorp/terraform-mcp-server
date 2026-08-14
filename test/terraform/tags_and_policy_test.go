package terraform

import (
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

		assert.Equal(t, ps.ID, gjson.Get(resultText, "0.id").String(), "response should contain the policy set ID")
		assert.Equal(t, psName, gjson.Get(resultText, "0.name").String(), "response should contain the policy set name")
		assert.Equal(t, "directly attached", gjson.Get(resultText, "0.reason").String(), "policy set should be reported as directly attached")
		assert.False(t, gjson.Get(resultText, "0.global").Bool(), "policy set should not be global")
	})

	t.Run("returns no policy sets for an unattached workspace", func(t *testing.T) {
		// Create a second workspace that has no policy sets attached.
		bareWsName := randomName("ws-bare-")
		bareWs, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
			Name: &bareWsName,
		})
		require.NoError(t, err, "setup: failed to create bare workspace via TFE API")
		defer client.Workspaces.SafeDeleteByID(t.Context(), bareWs.ID)

		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       bareWs.ID,
		})

		require.False(t, result.IsError, "list_workspace_policy_sets should not return an error for an unattached workspace")
		assert.Contains(t, resultText, "No policy sets are attached to workspace", "response should indicate no policy sets are attached")
		assert.Contains(t, resultText, bareWs.ID, "response should reference the workspace ID")
	})
}

func TestListWorkspacePolicySetsErrorPaths(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	nonExistentOrg := randomName("org-")
	const nonExistentWsID = "ws-0000000000dead"

	t.Run("returns an error for a non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_id":       nonExistentWsID,
		})
		assert.True(t, result.IsError, "list_workspace_policy_sets should return an error for a non-existent org")
	})

	t.Run("returns no policy sets for a non-existent workspace ID in a valid org", func(t *testing.T) {
		// The tool lists org-level policy sets and filters by workspace ID match.
		// A bogus workspace ID will never match, so the tool returns a plain-text
		// "no policy sets" message rather than an API error.
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       nonExistentWsID,
		})
		assert.False(t, result.IsError, "list_workspace_policy_sets should not return an error for a non-existent workspace ID")
		assert.Contains(t, resultText, "No policy sets are attached to workspace", "response should indicate no policy sets are attached")
		assert.Contains(t, resultText, nonExistentWsID, "response should reference the workspace ID")
	})
}
