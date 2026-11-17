// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentTestLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestStatelessTfeClientIsCached(t *testing.T) {
	logger := silentTestLogger()

	t.Run("same token reuses the client and skips the ping", func(t *testing.T) {
		server, pings := fakeTerraform(t)
		first, err := GetTfeClientForSession(ctxWithAddress(server.URL, "token-a"), "", logger)
		require.NoError(t, err)
		second, err := GetTfeClientForSession(ctxWithAddress(server.URL, "token-a"), "", logger)
		require.NoError(t, err)
		assert.Same(t, first, second)
		assert.EqualValues(t, 1, pings.Load(), "the second request shouldn't build a new client")
	})

	t.Run("a different token gets its own client", func(t *testing.T) {
		server, pings := fakeTerraform(t)
		a, err := GetTfeClientForSession(ctxWithAddress(server.URL, "token-a"), "", logger)
		require.NoError(t, err)
		b, err := GetTfeClientForSession(ctxWithAddress(server.URL, "token-b"), "", logger)
		require.NoError(t, err)
		assert.NotSame(t, a, b, "a client must never go to a request with a different token")
		assert.EqualValues(t, 2, pings.Load())
	})

	t.Run("a different address gets its own client", func(t *testing.T) {
		serverA, _ := fakeTerraform(t)
		serverB, _ := fakeTerraform(t)
		a, err := GetTfeClientForSession(ctxWithAddress(serverA.URL, "token-a"), "", logger)
		require.NoError(t, err)
		b, err := GetTfeClientForSession(ctxWithAddress(serverB.URL, "token-a"), "", logger)
		require.NoError(t, err)
		assert.NotSame(t, a, b)
	})

	t.Run("no token is still an error and nothing gets cached", func(t *testing.T) {
		server, _ := fakeTerraform(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv(TerraformToken, "")
		before := statelessTfeClients.len()
		_, err := GetTfeClientForSession(ctxWithAddress(server.URL, ""), "", logger)
		require.ErrorContains(t, err, "no Terraform token provided")
		assert.Equal(t, before, statelessTfeClients.len())
	})
}

func TestTfeClientCacheKey(t *testing.T) {
	base := tfeClientCacheKey("https://app.terraform.io", false, "token")
	assert.Equal(t, base, tfeClientCacheKey("https://app.terraform.io", false, "token"))
	assert.NotEqual(t, base, tfeClientCacheKey("https://app.terraform.io", false, "other"))
	assert.NotEqual(t, base, tfeClientCacheKey("https://tfe.example.com", false, "token"))
	assert.NotEqual(t, base, tfeClientCacheKey("https://app.terraform.io", true, "token"))
	// The separators keep the address and token from bleeding into each other
	assert.NotEqual(t, tfeClientCacheKey("ab", false, "c"), tfeClientCacheKey("a", false, "bc"))
}

func TestTfeClientCacheBounds(t *testing.T) {
	key := func(i int) [32]byte { return tfeClientCacheKey("addr", false, string(rune('a'+i))) }

	t.Run("evicts the least recently used past the max size", func(t *testing.T) {
		cache := newTfeClientCache(2, time.Hour)
		c0, c1, c2 := &tfe.Client{}, &tfe.Client{}, &tfe.Client{}
		cache.add(key(0), c0)
		cache.add(key(1), c1)
		_, _ = cache.get(key(0)) // 0 is now more recent than 1
		cache.add(key(2), c2)

		assert.Equal(t, 2, cache.len())
		_, ok := cache.get(key(1))
		assert.False(t, ok, "1 was least recently used so it should be gone")
		got, ok := cache.get(key(0))
		assert.True(t, ok)
		assert.Same(t, c0, got)
	})

	t.Run("drops clients that have been idle too long", func(t *testing.T) {
		now := time.Now()
		cache := newTfeClientCache(10, time.Minute)
		cache.now = func() time.Time { return now }
		cache.add(key(0), &tfe.Client{})

		now = now.Add(2 * time.Minute)
		_, ok := cache.get(key(0))
		assert.False(t, ok)
		assert.Equal(t, 0, cache.len())
	})

	t.Run("using a client keeps it alive", func(t *testing.T) {
		now := time.Now()
		cache := newTfeClientCache(10, time.Minute)
		cache.now = func() time.Time { return now }
		cache.add(key(0), &tfe.Client{})

		for range 3 {
			now = now.Add(45 * time.Second)
			_, ok := cache.get(key(0))
			require.True(t, ok)
		}
	})

	t.Run("safe to use from many goroutines", func(t *testing.T) {
		cache := newTfeClientCache(5, time.Hour)
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.add(key(i%8), &tfe.Client{})
				_, _ = cache.get(key((i + 1) % 8))
			}()
		}
		wg.Wait()
		assert.LessOrEqual(t, cache.len(), 5)
	})
}

// A cached client gets reused by requests from different callers, so the
// X-Forwarded-For header has to come from each request, not the first one.
func TestCachedClientSendsEachRequestsClientIP(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/ping" {
			mu.Lock()
			seen = append(seen, r.Header.Get("X-Forwarded-For"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = io.WriteString(w, `{"data":{"id":"acme","type":"organizations","attributes":{"name":"acme"}}}`)
	}))
	t.Cleanup(server.Close)
	logger := silentTestLogger()

	for _, ip := range []string{"203.0.113.1", "203.0.113.2", ""} {
		ctx := ctxWithAddress(server.URL, "token-xff")
		if ip != "" {
			ctx = context.WithValue(ctx, contextKey(ClientIPKey), ip)
		}
		client, err := GetTfeClientForSession(ctx, "", logger)
		require.NoError(t, err)
		_, err = client.Organizations.Read(ctx, "acme")
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"203.0.113.1", "203.0.113.2", ""}, seen)
}

// Skipping the ping means a revoked token isn't caught when the client comes
// out of the cache, but the real API call still fails with a 401.
func TestCachedClientWithRevokedToken(t *testing.T) {
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if revoked.Load() && r.URL.Path != "/api/v2/ping" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"status":"401","title":"unauthorized"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"acme","type":"organizations","attributes":{"name":"acme"}}}`)
	}))
	t.Cleanup(server.Close)
	logger := silentTestLogger()
	ctx := ctxWithAddress(server.URL, "token-revoked")

	client, err := GetTfeClientForSession(ctx, "", logger)
	require.NoError(t, err)
	_, err = client.Organizations.Read(ctx, "acme")
	require.NoError(t, err)

	revoked.Store(true)
	cached, err := GetTfeClientForSession(ctx, "", logger)
	require.NoError(t, err)
	require.Same(t, client, cached)
	_, err = cached.Organizations.Read(ctx, "acme")
	assert.ErrorIs(t, err, tfe.ErrUnauthorized)
}

func TestOrganizationAllowlistUsesCachedClient(t *testing.T) {
	server, pings := fakeTerraform(t)
	t.Setenv(TerraformAddress, server.URL)
	logger := silentTestLogger()

	_, err := organizationListerForRequest(context.Background(), "token-allowlist", logger)
	require.NoError(t, err)
	_, err = organizationListerForRequest(context.Background(), "token-allowlist", logger)
	require.NoError(t, err)
	assert.EqualValues(t, 1, pings.Load(), "the allowlist check should reuse the cached client")

	// and the tool call right after uses the same client, so no extra ping there either
	_, err = GetTfeClientForSession(ctxWithAddress(server.URL, "token-allowlist"), "", logger)
	require.NoError(t, err)
	assert.EqualValues(t, 1, pings.Load())
}
