// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhoAmI(t *testing.T) {
	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "whoami", map[string]any{})

	require.False(t, result.IsError, "whoami should not return an error")
	require.NotEmpty(t, resultText, "whoami should return a non-empty response")

	var toolAccount struct {
		Username         string `json:"username"`
		Email            string `json:"email"`
		IsServiceAccount *bool  `json:"is_service_account"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText), &toolAccount), "whoami should return valid account details")

	tfeUser, err := client.Users.ReadCurrent(t.Context())
	require.NoError(t, err, "failed to read the current user directly from TFE")
	assert.Equal(t, tfeUser.Username, toolAccount.Username)
	assert.Equal(t, tfeUser.Email, toolAccount.Email)
	if assert.NotNil(t, toolAccount.IsServiceAccount) {
		assert.Equal(t, tfeUser.IsServiceAccount, *toolAccount.IsServiceAccount)
	}
}

func TestGetTokenPermissions(t *testing.T) {
	client := tfeClient(t)
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "get_token_permissions", map[string]any{
		"terraform_org_name": tfeOrgName,
	})

	require.False(t, result.IsError, "get_token_permissions should not return an error")
	require.NotEmpty(t, resultText, "get_token_permissions should return a non-empty response")

	var toolPermissions []string
	require.NoError(t, json.Unmarshal([]byte(resultText), &toolPermissions), "get_token_permissions should return a JSON array of strings")

	tfeOrg, err := client.Organizations.Read(t.Context(), tfeOrgName)
	require.NoError(t, err, "failed to read the organization directly from TFE")

	tfePermissions := map[string]bool{
		"Create Teams":                  tfeOrg.Permissions.CanCreateTeam,
		"Create Workspaces":             tfeOrg.Permissions.CanCreateWorkspace,
		"Create Workspace Migrations":   tfeOrg.Permissions.CanCreateWorkspaceMigration,
		"Deploy NoCode Modules":         tfeOrg.Permissions.CanDeployNoCodeModules,
		"Destroy":                       tfeOrg.Permissions.CanDestroy,
		"Manage Auditing":               tfeOrg.Permissions.CanManageAuditing,
		"Manage NoCodeModules":          tfeOrg.Permissions.CanManageNoCodeModules,
		"Manage Run Tasks":              tfeOrg.Permissions.CanManageRunTasks,
		"Traverse":                      tfeOrg.Permissions.CanTraverse,
		"Update":                        tfeOrg.Permissions.CanUpdate,
		"Update API Tokens":             tfeOrg.Permissions.CanUpdateAPIToken,
		"Update OAuth":                  tfeOrg.Permissions.CanUpdateOAuth,
		"Update Sentinel":               tfeOrg.Permissions.CanUpdateSentinel,
		"Update HYOK Configuration":     tfeOrg.Permissions.CanUpdateHYOKConfiguration,
		"View HYOK Feature Information": tfeOrg.Permissions.CanViewHYOKFeatureInfo,
		"Enable Stacks":                 tfeOrg.Permissions.CanEnableStacks,
		"Create Projects":               tfeOrg.Permissions.CanCreateProject,
	}

	var expectedPermissions []string
	for name, allowed := range tfePermissions {
		if allowed {
			expectedPermissions = append(expectedPermissions, name)
		}
	}
	assert.ElementsMatch(t, expectedPermissions, toolPermissions, "tool permissions should match the TFE API")
}
