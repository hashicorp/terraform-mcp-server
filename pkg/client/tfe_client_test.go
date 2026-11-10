// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
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

// fakeTerraform stands in for HCPT and counts the /api/v2/ping calls
// tfe.NewClient makes, so tests can see when a client gets rebuilt.
func fakeTerraform(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var pings atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			pings.Add(1)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(server.Close)
	return server, &pings
}

// useCredentialsFile sets HOME to a temp dir with a credentials.tfrc.json that
// has the token for address, and clears TFE_TOKEN so the file is the only
// other place to get a token from.
func useCredentialsFile(t *testing.T, address, token string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".terraform.d")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := fmt.Sprintf(`{"credentials":{%q:{"token":%q}}}`, extractHostname(address), token)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials.tfrc.json"), []byte(content), 0o600))
	t.Setenv("HOME", home)
	t.Setenv(TerraformToken, "")
}

func ctxWithAddress(address, token string) context.Context {
	ctx := context.WithValue(context.Background(), contextKey(TerraformAddress), address)
	if token != "" {
		ctx = context.WithValue(ctx, contextKey(TerraformToken), token)
	}
	return ctx
}

func TestGetTfeClientForSessionWithCredentialsFile(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("cached client is reused when the token comes from the credentials file", func(t *testing.T) {
		server, pings := fakeTerraform(t)
		useCredentialsFile(t, server.URL, "file-token")
		sessionID := "session-credentials-file"
		t.Cleanup(func() { DeleteTfeClient(sessionID) })
		ctx := ctxWithAddress(server.URL, "")

		// Session start builds the client, similar to what NewSessionHandler does
		created, err := CreateTfeClientForSession(ctx, sessionID, logger)
		require.NoError(t, err)
		require.EqualValues(t, 1, pings.Load())

		for i := 0; i < 3; i++ {
			got, err := GetTfeClientForSession(ctx, sessionID, logger)
			require.NoError(t, err)
			assert.Same(t, created, got, "tool calls should reuse the session's client")
		}
		assert.EqualValues(t, 1, pings.Load(), "reusing the client shouldn't ping again")
	})

	t.Run("stateless mode works with only a credentials file token", func(t *testing.T) {
		server, _ := fakeTerraform(t)
		useCredentialsFile(t, server.URL, "file-token")

		client, err := GetTfeClientForSession(ctxWithAddress(server.URL, ""), "", logger)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("a header token still wins over the credentials file", func(t *testing.T) {
		server, pings := fakeTerraform(t)
		useCredentialsFile(t, server.URL, "file-token")
		sessionID := "session-header-over-file"
		t.Cleanup(func() { DeleteTfeClient(sessionID) })

		fromHeader, err := GetTfeClientForSession(ctxWithAddress(server.URL, "header-token"), sessionID, logger)
		require.NoError(t, err)
		again, err := GetTfeClientForSession(ctxWithAddress(server.URL, "header-token"), sessionID, logger)
		require.NoError(t, err)
		assert.Same(t, fromHeader, again)
		assert.EqualValues(t, 1, pings.Load())

		// Dropping the header falls back to the file token, which is a different
		// token, so the client can get rebuilt
		fromFile, err := GetTfeClientForSession(ctxWithAddress(server.URL, ""), sessionID, logger)
		require.NoError(t, err)
		assert.NotSame(t, fromHeader, fromFile)
		assert.EqualValues(t, 2, pings.Load())
	})

	t.Run("no token anywhere is still an error", func(t *testing.T) {
		server, _ := fakeTerraform(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv(TerraformToken, "")
		sessionID := "session-no-token"
		t.Cleanup(func() { DeleteTfeClient(sessionID) })

		_, err := GetTfeClientForSession(ctxWithAddress(server.URL, ""), sessionID, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Terraform token provided")

		_, err = GetTfeClientForSession(ctxWithAddress(server.URL, ""), "", logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Terraform token provided")
	})
}

func TestTerraformTokenForRequest(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	const address = "https://tfe.example.com"

	tests := []struct {
		name     string
		ctxToken string
		envToken string
		file     bool
		want     string
	}{
		{name: "ctx token first", ctxToken: "ctx-token", envToken: "env-token", file: true, want: "ctx-token"},
		{name: "then env", envToken: "env-token", file: true, want: "env-token"},
		{name: "then credentials file", file: true, want: "file-token"},
		{name: "nothing", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.file {
				useCredentialsFile(t, address, "file-token")
			} else {
				t.Setenv("HOME", t.TempDir())
			}
			t.Setenv(TerraformToken, tt.envToken)

			got, err := terraformTokenForRequest(ctxWithAddress(address, tt.ctxToken), address, logger)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTerraformTokenForRequestCredentialsFileErrors(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	const address = "https://tfe.example.com"

	writeFile := func(t *testing.T, content string, mode os.FileMode) {
		t.Helper()
		home := t.TempDir()
		dir := filepath.Join(home, ".terraform.d")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials.tfrc.json"), []byte(content), mode))
		t.Setenv("HOME", home)
		t.Setenv(TerraformToken, "")
	}

	t.Run("no entry for this host is just no token", func(t *testing.T) {
		writeFile(t, `{"credentials":{"app.terraform.io":{"token":"other"}}}`, 0o600)
		token, err := terraformTokenForRequest(ctxWithAddress(address, ""), address, logger)
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("malformed file returns the parse error", func(t *testing.T) {
		writeFile(t, `{not json`, 0o600)
		_, err := terraformTokenForRequest(ctxWithAddress(address, ""), address, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading credentials.tfrc.json")

		// and it comes back from building the session client too, instead of
		// "no Terraform token provided"
		_, err = CreateTfeClientForSession(ctxWithAddress(address, ""), "session-bad-file", logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading credentials.tfrc.json")
		assert.NotContains(t, err.Error(), "no Terraform token provided")
	})

	t.Run("unreadable file returns the permission error", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can read the file anyway")
		}
		writeFile(t, `{"credentials":{"tfe.example.com":{"token":"file-token"}}}`, 0o000)
		_, err := terraformTokenForRequest(ctxWithAddress(address, ""), address, logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, fs.ErrPermission)
	})
}
