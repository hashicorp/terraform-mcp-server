// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHttpClientForSession(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("different sessions get independently cached clients", func(t *testing.T) {
		sessionA := "http-session-a"
		sessionB := "http-session-b"
		t.Cleanup(func() {
			DeleteHttpClient(sessionA)
			DeleteHttpClient(sessionB)
		})

		clientA := GetHttpClientForSession(context.Background(), sessionA, logger)
		clientB := GetHttpClientForSession(context.Background(), sessionB, logger)
		require.NotNil(t, clientA)
		require.NotNil(t, clientB)
		assert.NotSame(t, clientA, clientB, "different sessions must not share a cached HTTP client")

		// Both clients must be reachable directly via the cache too.
		assert.Same(t, clientA, GetHttpClient(sessionA))
		assert.Same(t, clientB, GetHttpClient(sessionB))

		// A second call for session A must return the cached client, not rebuild it.
		clientAAgain := GetHttpClientForSession(context.Background(), sessionA, logger)
		assert.Same(t, clientA, clientAAgain)
	})

	t.Run("empty sessionID bypasses the cache entirely (stateless mode)", func(t *testing.T) {
		_, alreadyCached := activeHttpClients.Load("")
		require.False(t, alreadyCached, "test precondition: empty key must not already be cached")

		clientOne := GetHttpClientForSession(context.Background(), "", logger)
		clientTwo := GetHttpClientForSession(context.Background(), "", logger)
		require.NotNil(t, clientOne)
		require.NotNil(t, clientTwo)

		assert.NotSame(t, clientOne, clientTwo, "stateless requests must always build a fresh client, never share one")

		_, ok := activeHttpClients.Load("")
		assert.False(t, ok, "an empty sessionID must never be used as a cache key")
	})
}
