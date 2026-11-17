// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedTransport(t *testing.T) {
	verify := sharedTransport(false)
	skip := sharedTransport(true)

	assert.Same(t, verify, sharedTransport(false), "same TLS setting should get the same transport")
	assert.Same(t, skip, sharedTransport(true))
	assert.NotSame(t, verify, skip, "skip-verify needs its own transport since TLS config lives on it")

	assert.False(t, verify.TLSClientConfig.InsecureSkipVerify)
	assert.True(t, skip.TLSClientConfig.InsecureSkipVerify)

	for _, transport := range []*http.Transport{verify, skip} {
		assert.Equal(t, maxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
		assert.True(t, transport.ForceAttemptHTTP2, "a custom TLS config turns off HTTP/2 unless this is set")
		assert.Positive(t, transport.IdleConnTimeout, "idle connections should get closed eventually")
		assert.Positive(t, transport.TLSHandshakeTimeout)
		assert.NotNil(t, transport.Proxy)
	}
}

// Clients are built per session, and per request in stateless mode, so the
// connection has to be reused across separate clients, not just one client.
func TestCreateHTTPClientReusesConnectionsAcrossClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	get := func(client *http.Client) bool {
		t.Helper()
		var reused bool
		trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		// The body has to be read and closed for the connection to go back to the pool
		_, _ = io.Copy(io.Discard, resp.Body)
		require.NoError(t, resp.Body.Close())
		return reused
	}

	get(CreateHTTPClient(false, nil))
	assert.True(t, get(CreateHTTPClient(false, nil)), "a new client should reuse the connection the first one opened")
}
