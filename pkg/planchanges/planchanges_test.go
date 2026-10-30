// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package planchanges

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixture = `{
  "format_version": "1.2",
  "resource_changes": [
    {"address": "aws_s3_bucket.kept", "change": {"actions": ["no-op"], "before": {"bucket": "secret-name"}, "after": {"bucket": "secret-name"}}},
    {"address": "data.aws_caller_identity.current", "change": {"actions": ["read"]}},
    {"address": "aws_iam_role.new", "change": {"actions": ["create"], "after": {"name": "very-secret-value"}}},
    {"address": "aws_iam_role.gone", "change": {"actions": ["delete"], "before": {"name": "x"}}},
    {"address": "aws_instance.web", "change": {"actions": ["delete", "create"]}},
    {"address": "aws_instance.db", "change": {"actions": ["create", "delete"]}},
    {"address": "aws_sqs_queue.q", "change": {"actions": ["update"], "before_sensitive": {"policy": true}, "after": {"policy": "{\"Statement\":[]}"}}},
    {"address": "aws_vpc.main", "change": {"actions": ["no-op"], "importing": {"id": "vpc-123"}}},
    {"address": "aws_subnet.a", "change": {"actions": ["update"], "importing": {"id": "subnet-1"}}},
    {"address": "module.net.aws_route_table.rt", "previous_address": "aws_route_table.rt", "change": {"actions": ["no-op"]}},
    {"address": "aws_kms_key.k", "change": {"actions": ["forget"]}}
  ],
  "resource_drift": [
    {"address": "aws_iam_role.gone", "change": {"actions": ["update"], "after": {"tags": {"Owner": "someone"}}}},
    {"address": "aws_s3_bucket.kept", "change": {"actions": ["no-op"]}}
  ],
  "output_changes": {
    "z_url": {"actions": ["update"], "after": "https://private.example"},
    "a_id": {"actions": ["create"]},
    "unchanged": {"actions": ["no-op"]}
  }
}`

func TestSummarize(t *testing.T) {
	s, err := Summarize([]byte(fixture))
	require.NoError(t, err)

	assert.Equal(t, []ResourceChange{
		{Action: "delete", Address: "aws_iam_role.gone"},
		{Action: "replace", Address: "aws_instance.db"},
		{Action: "replace", Address: "aws_instance.web"},
		{Action: "forget", Address: "aws_kms_key.k"},
		{Action: "update", Address: "aws_sqs_queue.q"},
		{Action: "update", Address: "aws_subnet.a", Importing: true},
		{Action: "create", Address: "aws_iam_role.new"},
		{Action: "import", Address: "aws_vpc.main"},
		{Action: "move", Address: "module.net.aws_route_table.rt", PreviousAddress: "aws_route_table.rt"},
	}, s.ResourceChanges)

	assert.Equal(t, map[string]int{
		"delete": 1, "replace": 2, "forget": 1, "update": 2, "create": 1, "import": 1, "move": 1,
	}, s.Counts)

	assert.Equal(t, []ResourceChange{{Action: "update", Address: "aws_iam_role.gone"}}, s.ResourceDrift)
	assert.Equal(t, []OutputChange{{Action: "create", Name: "a_id"}, {Action: "update", Name: "z_url"}}, s.OutputChanges)
	assert.False(t, s.Truncated)
}

func TestSummarizeNeverReturnsValues(t *testing.T) {
	s, err := Summarize([]byte(fixture))
	require.NoError(t, err)
	out, err := json.Marshal(s)
	require.NoError(t, err)

	for _, value := range []string{"secret-name", "very-secret-value", "Statement", "someone", "private.example", "vpc-123", "subnet-1"} {
		assert.NotContains(t, string(out), value)
	}
}

func TestSummarizeEmptyLists(t *testing.T) {
	s, err := Summarize([]byte(`{"resource_changes": [{"address": "a.b", "change": {"actions": ["no-op"]}}]}`))
	require.NoError(t, err)

	out, err := json.Marshal(s)
	require.NoError(t, err)
	assert.JSONEq(t, `{"counts": {}, "resource_changes": [], "resource_drift": [], "output_changes": [], "truncated": false}`, string(out))
}

func TestSummarizeTruncates(t *testing.T) {
	var rc []string
	for i := 0; i < MaxEntries+10; i++ {
		rc = append(rc, fmt.Sprintf(`{"address": "null_resource.r%04d", "change": {"actions": ["create"]}}`, i))
	}
	s, err := Summarize([]byte(`{"resource_changes": [` + strings.Join(rc, ",") + `]}`))
	require.NoError(t, err)

	assert.Len(t, s.ResourceChanges, MaxEntries)
	assert.Equal(t, MaxEntries+10, s.Counts["create"])
	assert.True(t, s.Truncated)
}

func TestSummarizeErrors(t *testing.T) {
	_, err := Summarize(nil)
	assert.ErrorIs(t, err, ErrNotReady)

	_, err = Summarize([]byte("not json"))
	assert.ErrorContains(t, err, "failed to parse plan JSON")
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *tfe.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := tfe.NewClient(&tfe.Config{Address: srv.URL, Token: "tfe-token", RetryServerErrors: false})
	require.NoError(t, err)
	return client
}

func TestFetchFollowsRedirectWithoutToken(t *testing.T) {
	var archiveAuth string
	archive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archiveAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(fixture))
	}))
	t.Cleanup(archive.Close)
	// A different host name than the API, as with HCP Terraform's archivist.
	archiveURL := strings.Replace(archive.URL, "127.0.0.1", "localhost", 1)

	var paths []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		assert.Equal(t, "Bearer tfe-token", r.Header.Get("Authorization"))
		http.Redirect(w, r, archiveURL+"/object", http.StatusTemporaryRedirect)
	})

	raw, err := Fetch(t.Context(), client, "run-abc123", "")
	require.NoError(t, err)
	assert.JSONEq(t, fixture, string(raw))

	_, err = Fetch(t.Context(), client, "", "plan-abc123")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"/api/v2/runs/run-abc123/plan/json-output-redacted",
		"/api/v2/plans/plan-abc123/json-output-redacted",
	}, paths)
	assert.Empty(t, archiveAuth, "the Terraform token must not be sent to the archive host")
}

func TestFetchPlanNotFinished(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	_, err := Fetch(t.Context(), client, "run-abc123", "")
	assert.ErrorIs(t, err, ErrNotReady)
}

func TestFetchValidatesInput(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	})

	for name, tc := range map[string]struct{ runID, planID, err string }{
		"neither":      {err: "one of run_id or plan_id is required"},
		"both":         {runID: "run-a", planID: "plan-b", err: "provide only one of run_id or plan_id"},
		"bad run id":   {runID: "run-a/../x", err: "invalid run_id"},
		"bad plan id":  {planID: "ws-123", err: "invalid plan_id"},
		"blank values": {runID: "  ", planID: " ", err: "one of run_id or plan_id is required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Fetch(t.Context(), client, tc.runID, tc.planID)
			assert.ErrorContains(t, err, tc.err)
		})
	}
}
