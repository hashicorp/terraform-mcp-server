// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"encoding/json"
	"testing"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
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
	assert.NotNil(t, toolAccount.IsServiceAccount)
	if toolAccount.IsServiceAccount != nil {
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

	expectedPermissions := tfeclient.HumanReadableTokenPermissions(tfeOrg.Permissions)
	assert.ElementsMatch(t, expectedPermissions, toolPermissions, "tool permissions should match the TFE API")
}
