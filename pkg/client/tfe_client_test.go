// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This tests the buildTFEConfig directly due to tfe.NewClient consuming the config and
// it oesn't give the headers back to assert on. The newTfeClient func calls this, so it covers the prod path.

func TestBuildTFEConfig_SharedSecret(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("sets shared secret header when env is set", func(t *testing.T) {
		t.Setenv(SharedSecretEnv, "super-secret-value")
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Equal(t, "super-secret-value", cfg.Headers.Get(SharedSecretHeader))
	})

	t.Run("omits shared secret header when env is unset", func(t *testing.T) {
		t.Setenv(SharedSecretEnv, "")
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Empty(t, cfg.Headers.Get(SharedSecretHeader))
	})
}

func TestBuildTFEConfig_ForwardedFor(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("sets X-Forwarded-For when clientIP is provided", func(t *testing.T) {
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "203.0.113.5", logger)
		assert.Equal(t, "203.0.113.5", cfg.Headers.Get("X-Forwarded-For"))
	})

	t.Run("omits X-Forwarded-For when clientIP is empty", func(t *testing.T) {
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Empty(t, cfg.Headers.Get("X-Forwarded-For"))
	})
}

func TestHumanReadableTokenPermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions tfe.OrganizationPermissions
		expected    string
	}{
		{"CanCreateTeam", tfe.OrganizationPermissions{CanCreateTeam: true}, "Create Teams"},
		{"CanCreateWorkspace", tfe.OrganizationPermissions{CanCreateWorkspace: true}, "Create Workspaces"},
		{"CanCreateWorkspaceMigration", tfe.OrganizationPermissions{CanCreateWorkspaceMigration: true}, "Create Workspace Migrations"},
		{"CanDeployNoCodeModules", tfe.OrganizationPermissions{CanDeployNoCodeModules: true}, "Deploy NoCode Modules"},
		{"CanDestroy", tfe.OrganizationPermissions{CanDestroy: true}, "Destroy"},
		{"CanManageAuditing", tfe.OrganizationPermissions{CanManageAuditing: true}, "Manage Auditing"},
		{"CanManageNoCodeModules", tfe.OrganizationPermissions{CanManageNoCodeModules: true}, "Manage NoCodeModules"},
		{"CanManageRunTasks", tfe.OrganizationPermissions{CanManageRunTasks: true}, "Manage Run Tasks"},
		{"CanTraverse", tfe.OrganizationPermissions{CanTraverse: true}, "Traverse"},
		{"CanUpdate", tfe.OrganizationPermissions{CanUpdate: true}, "Update"},
		{"CanUpdateAPIToken", tfe.OrganizationPermissions{CanUpdateAPIToken: true}, "Update API Tokens"},
		{"CanUpdateOAuth", tfe.OrganizationPermissions{CanUpdateOAuth: true}, "Update OAuth"},
		{"CanUpdateSentinel", tfe.OrganizationPermissions{CanUpdateSentinel: true}, "Update Sentinel"},
		{"CanUpdateHYOKConfiguration", tfe.OrganizationPermissions{CanUpdateHYOKConfiguration: true}, "Update HYOK Configuration"},
		{"CanViewHYOKFeatureInfo", tfe.OrganizationPermissions{CanViewHYOKFeatureInfo: true}, "View HYOK Feature Information"},
		{"CanEnableStacks", tfe.OrganizationPermissions{CanEnableStacks: true}, "Enable Stacks"},
		{"CanCreateProject", tfe.OrganizationPermissions{CanCreateProject: true}, "Create Projects"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, []string{tt.expected}, HumanReadableTokenPermissions(&tt.permissions))
		})
	}

	t.Run("nil permissions", func(t *testing.T) {
		assert.Nil(t, HumanReadableTokenPermissions(nil))
	})
}

// ctxWithTfeValues builds a context carrying the token/address the way
// TerraformContextMiddleware does, using this package's own contextKey type -
// the same type both the mark3labs and official-SDK transports read through.
func ctxWithTfeValues(token string) context.Context {
	ctx := context.WithValue(context.Background(), contextKey(TerraformAddress), "https://app.terraform.io")
	ctx = context.WithValue(ctx, contextKey(TerraformToken), token)
	return ctx
}

func TestGetTfeClientForSession(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("different sessions with different tokens get independently cached clients", func(t *testing.T) {
		sessionA := "session-a"
		sessionB := "session-b"
		t.Cleanup(func() {
			DeleteTfeClient(sessionA)
			DeleteTfeClient(sessionB)
		})

		clientA, err := GetTfeClientForSession(ctxWithTfeValues("token-a"), sessionA, logger)
		require.NoError(t, err)
		require.NotNil(t, clientA)

		clientB, err := GetTfeClientForSession(ctxWithTfeValues("token-b"), sessionB, logger)
		require.NoError(t, err)
		require.NotNil(t, clientB)

		assert.NotSame(t, clientA, clientB, "different sessions must not share a cached client")

		// Each session's client must be reachable directly via the cache too.
		v, ok := activeTfeClients.Load(sessionA)
		require.True(t, ok)
		assert.Same(t, clientA, v.(cachedTfeClient).client)

		v, ok = activeTfeClients.Load(sessionB)
		require.True(t, ok)
		assert.Same(t, clientB, v.(cachedTfeClient).client)

		// A second call for session A with the same token must return the cached client, not rebuild it.
		clientAAgain, err := GetTfeClientForSession(ctxWithTfeValues("token-a"), sessionA, logger)
		require.NoError(t, err)
		assert.Same(t, clientA, clientAAgain)
	})

	t.Run("token change on the same session invalidates the cached client", func(t *testing.T) {
		sessionID := "session-rotate"
		t.Cleanup(func() { DeleteTfeClient(sessionID) })

		original, err := GetTfeClientForSession(ctxWithTfeValues("token-v1"), sessionID, logger)
		require.NoError(t, err)
		require.NotNil(t, original)

		rotated, err := GetTfeClientForSession(ctxWithTfeValues("token-v2"), sessionID, logger)
		require.NoError(t, err)
		require.NotNil(t, rotated)

		assert.NotSame(t, original, rotated, "a token change must invalidate the previously cached client")

		v, ok := activeTfeClients.Load(sessionID)
		require.True(t, ok)
		assert.Same(t, rotated, v.(cachedTfeClient).client, "the cache must hold the newest client after rotation")
	})
}
