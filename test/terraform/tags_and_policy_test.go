package terraform

import (
	"fmt"
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
}

func TestListWorkspacePolicySetsErrorPaths(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	nonExistentOrg := "org-doesnotexist123"
	nonExistentWsID := "ws-doesnotexist123"

	t.Run("returns an error for a non-existent org", func(t *testing.T) {
		result, _ := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": nonExistentOrg,
			"workspace_id":       nonExistentWsID,
		})
		assert.True(t, result.IsError, "list_workspace_policy_sets should return an error for a non-existent org")
	})

	t.Run("does not error for a non-existent workspace ID in a valid org", func(t *testing.T) {
		// The tool never validates whether workspace_id exists in the API — it uses it
		// only as a client-side filter against directly-attached policy sets. Global
		// policy sets are always returned regardless. The response content therefore
		// depends on live org state and cannot be deterministically asserted here.
		result, resultText := callTool(t, s, "list_workspace_policy_sets", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_id":       nonExistentWsID,
		})
		assert.False(t, result.IsError, "list_workspace_policy_sets should not return an error for a non-existent workspace ID")
		assert.Contains(t, resultText, "No policy sets are attached to workspace", "response should indicate no policy sets are attached")
	})
}
