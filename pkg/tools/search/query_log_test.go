// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/require"
)

func TestFastQueryLogBoundsAndIncompleteEvidence(t *testing.T) {
	log := "\x02" + strings.Repeat("x", maxQueryEvidenceBytes+1) + "\x03"
	var requests int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/logs":
			requests++
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(log) {
				_, _ = w.Write([]byte(log[offset:min(len(log), offset+limit)]))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	data, err := readFastQueryLog(context.Background(), c, backend.URL+"/logs?X-Amz-Signature=secret-sentinel")
	require.Nil(t, data)
	require.EqualError(t, err, "query_evidence_size_limit")
	require.Equal(t, 1, requests)

	log = "\x02" + strings.Repeat("x", 2048) // Missing ETX is not a complete log.
	data, err = readFastQueryLog(context.Background(), c, backend.URL+"/logs")
	require.Nil(t, data)
	require.EqualError(t, err, "query_log_unavailable")

	for _, location := range []string{"", "http://example.invalid/log", "https://user:pass@example.invalid/log", "https://example.invalid/log#fragment"} {
		data, err = readFastQueryLog(context.Background(), c, location)
		require.Nil(t, data)
		require.EqualError(t, err, "query_log_unavailable", "location must not escape: %s", location)
	}
}

func TestFastQueryLogStopsAfterBoundedRequests(t *testing.T) {
	var requests int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/logs":
			requests++
			_, _ = fmt.Fprint(w, "\x02")
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	data, err := readFastQueryLog(context.Background(), c, backend.URL+"/logs")
	require.Nil(t, data)
	require.EqualError(t, err, "query_log_unavailable")
	require.Equal(t, maxQueryLogRequests, requests)
}
