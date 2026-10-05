// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"testing"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests prove the delegation from pkg/mcp-official/client to pkg/client
// actually wires through: distinct sessionIDs must not collide, and the same
// sessionID must consistently hit the shared cache in pkg/client.
//
// The token/address are supplied via env vars (TerraformToken/TerraformAddress
// are exported from pkg/client) rather than context values, since pkg/client's
// contextKey type is unexported and unreachable from this package - exactly
// the boundary GetTfeClientForSession/GetHttpClientForSession are meant to
// cross on behalf of any transport.

func TestGetTfeClient_DelegatesPerSession(t *testing.T) {
	t.Setenv(tfeclient.TerraformToken, "delegation-test-token")
	t.Setenv(tfeclient.TerraformAddress, "https://app.terraform.io")

	sessionA := "official-tfe-session-a"
	sessionB := "official-tfe-session-b"
	t.Cleanup(func() {
		tfeclient.DeleteTfeClient(sessionA)
		tfeclient.DeleteTfeClient(sessionB)
	})

	clientA, err := GetTfeClient(context.Background(), sessionA)
	require.NoError(t, err)
	require.NotNil(t, clientA)

	clientB, err := GetTfeClient(context.Background(), sessionB)
	require.NoError(t, err)
	require.NotNil(t, clientB)

	assert.NotSame(t, clientA, clientB, "different sessionIDs must get independently cached TFE clients")

	clientAAgain, err := GetTfeClient(context.Background(), sessionA)
	require.NoError(t, err)
	assert.Same(t, clientA, clientAAgain, "the same sessionID must return the client cached by pkg/client")
}

func TestGetHttpClient_DelegatesPerSession(t *testing.T) {
	sessionA := "official-http-session-a"
	sessionB := "official-http-session-b"
	t.Cleanup(func() {
		tfeclient.DeleteHttpClient(sessionA)
		tfeclient.DeleteHttpClient(sessionB)
	})

	clientA, err := GetHttpClient(context.Background(), sessionA)
	require.NoError(t, err)
	require.NotNil(t, clientA)

	clientB, err := GetHttpClient(context.Background(), sessionB)
	require.NoError(t, err)
	require.NotNil(t, clientB)

	assert.NotSame(t, clientA, clientB, "different sessionIDs must get independently cached HTTP clients")

	clientAAgain, err := GetHttpClient(context.Background(), sessionA)
	require.NoError(t, err)
	assert.Same(t, clientA, clientAAgain, "the same sessionID must return the client cached by pkg/client")
}
