// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialStatusBackendOwnershipAndSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name, org, workspaceID, code                      string
		missingRelationship, missingWorkspace, unreadable bool
	}{
		{name: "allowed", org: "allowed", workspaceID: "ws-source"},
		{name: "denied", org: "blocked-secret", workspaceID: "ws-source", code: "query_run_unavailable"},
		{name: "missing relationship", missingRelationship: true, code: "query_ownership_unverified"},
		{name: "missing workspace", missingWorkspace: true, code: "query_ownership_unverified"},
		{name: "mismatched workspace", org: "allowed", workspaceID: "ws-other", code: "query_ownership_unverified"},
		{name: "missing organization", workspaceID: "ws-source", code: "query_ownership_unverified"},
		{name: "unreadable query", unreadable: true, code: "query_run_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var workspaceReads atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/queries/qry-secret-sentinel":
					if test.unreadable {
						http.Error(w, "backend-secret-sentinel", http.StatusNotFound)
						return
					}
					relationship := `,"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-source"}}}`
					if test.missingRelationship {
						relationship = ""
					}
					_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-secret-sentinel","attributes":{"status":"finished"}%s}}`, relationship)
				case "/api/v2/workspaces/ws-source":
					workspaceReads.Add(1)
					if test.missingWorkspace {
						http.Error(w, "backend-secret-sentinel", http.StatusNotFound)
						return
					}
					_, _ = fmt.Fprintf(w, `{"data":{"type":"workspaces","id":%q,"relationships":{"organization":{"data":{"type":"organizations","id":%q}}}}}`, test.workspaceID, test.org)
				default:
					t.Errorf("unexpected access: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed"})
			result, output, err := GetQueryStatusFunc(logger)(ctx, nil, QueryRunArguments{QueryRunID: "qry-secret-sentinel"})
			if test.code != "" {
				require.ErrorContains(t, err, test.code)
				require.Nil(t, result)
				require.Nil(t, output)
				require.NotContains(t, err.Error(), "secret")
				require.NotContains(t, err.Error(), backend.URL)
				if test.name == "denied" {
					require.ErrorIs(t, err, terraformclient.ErrOrganizationNotAllowed)
				}
			} else {
				require.NoError(t, err)
				require.True(t, output.Terminal)
				require.Equal(t, int32(1), workspaceReads.Load())
			}
			require.Contains(t, logs.String(), "poll_count")
			require.Contains(t, logs.String(), "elapsed")
			require.NotContains(t, logs.String(), "secret")
			require.NotContains(t, logs.String(), backend.URL)
		})
	}
}

func TestOfficialStatusPendingBudgetAndCallerCancellation(t *testing.T) {
	for _, mode := range []string{"budget", "deadline", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var reads atomic.Int32
			cause := errors.New("caller-secret-sentinel")
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			if mode == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 200*time.Millisecond)
				defer stop()
			}
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/queries/qry-pending-secret":
					reads.Add(1)
					if mode == "canceled" {
						cancel(cause)
					}
					_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-pending-secret","attributes":{"status":"running"}}}`)
				default:
					t.Errorf("unexpected backend access: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(backend.Close)
			t.Setenv("TFE_ADDRESS", backend.URL)
			t.Setenv("TFE_TOKEN", "test-token")
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			start := time.Now()
			result, output, err := GetQueryStatusFunc(logger)(ctx, nil, QueryRunArguments{QueryRunID: "qry-pending-secret"})
			if mode == "budget" {
				require.NoError(t, err)
				require.GreaterOrEqual(t, time.Since(start), 20*time.Second)
				require.Less(t, time.Since(start), 25*time.Second)
				require.False(t, output.Terminal)
				require.Equal(t, 5, output.RetryAfterSeconds)
				require.Contains(t, output.Message, "same query_run_id")
				require.Contains(t, output.Message, "do not execute another query")
				require.Greater(t, reads.Load(), int32(1))
				_, fallback, ok := strings.Cut(result.Content[0].(*mcp.TextContent).Text, "\n\n")
				require.True(t, ok)
				encoded, err := json.Marshal(output)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), fallback)
				require.Contains(t, logs.String(), "internal_budget")
			} else {
				require.ErrorContains(t, err, "query_run_unavailable")
				require.Nil(t, result)
				require.Nil(t, output)
				if mode == "canceled" {
					require.ErrorIs(t, err, cause)
				} else {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
				require.Less(t, time.Since(start), 3*time.Second)
				require.NotContains(t, err.Error(), "secret")
			}
			require.Contains(t, logs.String(), "poll_count")
			require.Contains(t, logs.String(), "elapsed")
			require.NotContains(t, logs.String(), "secret")
			require.NotContains(t, logs.String(), backend.URL)
		})
	}
}
