// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireOrgAllowlistScenario skips unless MCP_ORGANIZATION_ALLOWLIST is set to a
// list that excludes TFE_ORG_NAME and TFE_TOKEN can still reach an allowlisted org.
// This must run against its own server: setting the allowlist on the shared hcpt
// server would make every other hcpt test targeting TFE_ORG_NAME fail.
func requireOrgAllowlistScenario(t *testing.T, client *tfe.Client) {
	t.Helper()

	allowlist := strings.TrimSpace(os.Getenv("MCP_ORGANIZATION_ALLOWLIST"))
	if allowlist == "" {
		t.Skip("requires MCP_ORGANIZATION_ALLOWLIST (matching the server) and a TFE_TOKEN with access to an allowlisted organization")
	}

	allowed := make(map[string]struct{})
	for _, name := range strings.Split(allowlist, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if name == strings.ToLower(tfeOrgName) {
			t.Skipf("MCP_ORGANIZATION_ALLOWLIST %q must not include TFE_ORG_NAME (%q)", allowlist, tfeOrgName)
		}
		allowed[name] = struct{}{}
	}

	orgs, err := client.Organizations.List(t.Context(), &tfe.OrganizationListOptions{
		ListOptions: tfe.ListOptions{PageSize: 100},
	})
	require.NoError(t, err, "failed to list organizations for the configured TFE_TOKEN")
	for _, org := range orgs.Items {
		if _, ok := allowed[strings.ToLower(org.Name)]; ok {
			return
		}
	}
	t.Skipf("TFE_TOKEN cannot reach any organization in MCP_ORGANIZATION_ALLOWLIST (%q); use a token with access to both %q and an allowlisted organization", allowlist, tfeOrgName)
}

func TestOrganizationAllowlistEnforcement(t *testing.T) {
	requireTfOperations(t)

	client := tfeClient(t)
	requireOrgAllowlistScenario(t, client)
	requireTeamsEntitlement(t, client)
	requirePolicySetsEntitlement(t, client)

	s := newTestingSession(t)
	defer s.Close()

	// Create every fixture directly via the TFE API so the tool call is the only
	// operation under test.
	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name: tfe.String(randomName("allowlist-ws-")),
	})
	require.NoError(t, err, "setup: failed to create workspace via TFE API")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
		Name: randomName("allowlist-prj-"),
	})
	require.NoError(t, err, "setup: failed to create project via TFE API")
	defer client.Projects.Delete(t.Context(), project.ID)

	team, err := client.Teams.Create(t.Context(), tfeOrgName, tfe.TeamCreateOptions{
		Name: tfe.String(randomName("allowlist-team-")),
	})
	require.NoError(t, err, "setup: failed to create team via TFE API")
	defer client.Teams.Delete(t.Context(), team.ID)

	varSet, err := client.VariableSets.Create(t.Context(), tfeOrgName, &tfe.VariableSetCreateOptions{
		Name:   tfe.String(randomName("allowlist-vs-")),
		Global: tfe.Bool(false),
	})
	require.NoError(t, err, "setup: failed to create variable set via TFE API")
	defer client.VariableSets.Delete(t.Context(), varSet.ID)

	variable, err := client.VariableSetVariables.Create(t.Context(), varSet.ID, &tfe.VariableSetVariableCreateOptions{
		Key:      tfe.String("allowlist_test_key"),
		Value:    tfe.String("value"),
		Category: tfe.Category(tfe.CategoryTerraform),
	})
	require.NoError(t, err, "setup: failed to create variable in variable set via TFE API")

	policySet, err := client.PolicySets.Create(t.Context(), tfeOrgName, tfe.PolicySetCreateOptions{
		Name: tfe.String(randomName("allowlist-ps-")),
	})
	require.NoError(t, err, "setup: failed to create policy set via TFE API")
	defer client.PolicySets.Delete(t.Context(), policySet.ID)

	uploadConfiguration(t, client, workspace.ID, runTestConfiguration)
	runMessage := "Created by terraform-mcp-server organization allowlist integration tests"
	run, err := client.Runs.Create(t.Context(), tfe.RunCreateOptions{
		Workspace: workspace,
		AutoApply: tfe.Bool(false),
		Message:   &runMessage,
	})
	require.NoError(t, err, "setup: failed to create run via TFE API")

	// Every call must be rejected because its target belongs to TFE_ORG_NAME.
	// Destructive tools are listed last so a regression surfaces as a clear list
	// of leaked tools rather than a cascade of "not found".
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"force_unlock_workspace", "force_unlock_workspace", map[string]any{"workspace_id": workspace.ID}},
		{"action_run", "action_run", map[string]any{"run_action": "cancel", "run_id": run.ID}},
		{"grant_team_access (workspace)", "grant_team_access", map[string]any{"team_id": team.ID, "access_level": "read", "workspace_id": workspace.ID}},
		{"grant_team_access (project)", "grant_team_access", map[string]any{"team_id": team.ID, "access_level": "read", "project_id": project.ID}},
		{"add_team_member", "add_team_member", map[string]any{"team_id": team.ID, "username": randomName("allowlist-user-")}},
		{"get_state_version", "get_state_version", map[string]any{"workspace_id": workspace.ID}},
		{"create_variable_in_variable_set", "create_variable_in_variable_set", map[string]any{"variable_set_id": varSet.ID, "key": "blocked", "value": "v"}},
		{"delete_variable_in_variable_set", "delete_variable_in_variable_set", map[string]any{"variable_set_id": varSet.ID, "variable_id": variable.ID}},
		{"attach_variable_set_to_workspaces", "attach_variable_set_to_workspaces", map[string]any{"variable_set_id": varSet.ID, "workspace_ids": workspace.ID}},
		{"detach_variable_set_from_workspaces", "detach_variable_set_from_workspaces", map[string]any{"variable_set_id": varSet.ID, "workspace_ids": workspace.ID}},
		{"attach_policy_set_to_workspaces", "attach_policy_set_to_workspaces", map[string]any{"policy_set_id": policySet.ID, "workspace_ids": workspace.ID}},
		{"delete_workspace_safely", "delete_workspace_safely", map[string]any{"workspace_id": workspace.ID}},
		{"delete_project", "delete_project", map[string]any{"project_id": project.ID}},
		{"delete_team", "delete_team", map[string]any{"team_id": team.ID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, resultText := callTool(t, s, tc.tool, tc.args)
			require.True(t, result.IsError, "%s must be rejected by the organization allowlist", tc.tool)
			assert.Contains(t, resultText, "is not allowed by this server", "%s rejection should cite the allowlist", tc.tool)
			assert.Contains(t, resultText, tfeOrgName, "%s rejection should name the owning organization", tc.tool)
		})
	}
}
