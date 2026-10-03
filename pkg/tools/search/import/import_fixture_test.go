// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared source-backed mock material, not a live Atlas response or a server
// configuration-file handoff.
func phase0Fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../testdata/import/same-version", name))
	require.NoError(t, err)
	return b
}

func silentLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

type importBackendTest struct {
	client           *tfe.Client
	url              string
	mu               sync.Mutex
	requests         map[string]int
	responses        map[string]json.RawMessage
	schemaStatus     int
	cvDownloadStatus int
	stateStatus      int
	stateChanged     bool
	deniedDownloads  int
	queryLog         []byte
	baselineChanged  bool
	// changeReads is how many reads of the workspace or state see the original
	// before baselineChanged/stateChanged take effect. Zero means one.
	changeReads     int
	mutationHandler func(http.ResponseWriter, *http.Request) bool
}

func (f *importBackendTest) changeAfter() int {
	if f.changeReads > 0 {
		return f.changeReads
	}
	return 1
}

func importBackendFixture(t *testing.T) *importBackendTest {
	t.Helper()
	f := &importBackendTest{requests: map[string]int{}, schemaStatus: 200, cvDownloadStatus: http.StatusFound, stateStatus: 200, queryLog: phase0Fixture(t, "query.ndjson")}
	require.NoError(t, json.Unmarshal(phase0Fixture(t, "backend.json"), &f.responses))
	f.responses["/api/v2/workspaces/ws-fixture"] = f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"]
	f.responses["/schema-download"] = phase0Fixture(t, "provider-schema.json")
	// Simulated state metadata: contents/download URLs must not be consumed.
	f.responses["/api/v2/workspaces/ws-fixture/current-state-version"] = json.RawMessage(`{"data":{"type":"state-versions","id":"sv-current","attributes":{"serial":42,"hosted-state-download-url":"https://must-not-download.invalid/STATE-SECRET","providers":{"provider[\"registry.terraform.io/hashicorp/aws\"]":{"aws_iam_role":1}}},"relationships":{"run":{"data":{"type":"runs","id":"run-schema"}}}}}`)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		f.requests[key]++
		if f.mutationHandler != nil && f.mutationHandler(w, r) {
			return
		}
		assert.Equal(t, http.MethodGet, r.Method, "preparation must not mutate the backend")
		if r.URL.Path == "/api/v2/ping" {
			return
		}
		if r.URL.Path == "/logs" {
			log := append([]byte{2}, f.queryLog...)
			log = append(log, 3)
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(log) {
				_, _ = w.Write(log[offset:min(offset+limit, len(log))])
			}
			return
		}
		assert.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.URL.Path == "/schema-download" {
			assert.Equal(t, "fixture", r.URL.Query().Get("signed"))
			if f.requests[key] <= f.deniedDownloads {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"errors":[{"title":"expired signed URL"}]}`)
				return
			}
		}
		if r.URL.Path == "/api/v2/runs/run-schema/plan/json-schema" {
			if f.schemaStatus == 200 {
				http.Redirect(w, r, f.url+"/schema-download?signed=fixture", http.StatusTemporaryRedirect)
			} else {
				w.WriteHeader(f.schemaStatus)
				if f.schemaStatus >= 400 {
					_, _ = io.WriteString(w, `{"errors":[{"title":"denied secret must not leak"}]}`)
				}
			}
			return
		}
		if r.URL.Path == "/api/v2/configuration-versions/cv-current/download" {
			if f.cvDownloadStatus == http.StatusFound {
				http.Redirect(w, r, f.url+"/cv-archive?signed=fixture", http.StatusFound)
			} else {
				w.WriteHeader(f.cvDownloadStatus)
			}
			return
		}
		if r.URL.Path == "/cv-archive" {
			t.Error("the MCP server must not follow the configuration archive redirect")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/current-state-version") && f.stateStatus != 200 {
			w.WriteHeader(f.stateStatus)
			_, _ = io.WriteString(w, `{"errors":[{"title":"state unavailable"}]}`)
			return
		}
		body, ok := f.responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected API call: %s", key)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/v2/queries/qry-fixture" {
			body = []byte(strings.Replace(string(body), `"generate-config-out": true`, fmt.Sprintf(`"generate-config-out": true, "log-read-url": %q`, f.url+"/logs"), 1))
		}
		if f.baselineChanged && key == "GET /api/v2/organizations/fixture-org/workspaces/import-root" && f.requests[key] > f.changeAfter() {
			body = []byte(strings.ReplaceAll(string(body), "cv-current", "cv-changed"))
		}
		if f.stateChanged && strings.HasSuffix(r.URL.Path, "/current-state-version") && f.requests[key] > f.changeAfter() {
			body = []byte(strings.ReplaceAll(string(body), "sv-current", "sv-changed"))
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	f.url = s.URL
	c, err := tfe.NewClient(&tfe.Config{Address: s.URL, Token: "fixture-token", HTTPClient: s.Client()})
	require.NoError(t, err)
	f.client = c
	return f
}

func importFixtureInput(t *testing.T) importPrepareInput {
	t.Helper()
	var input importPrepareInput
	require.NoError(t, json.Unmarshal([]byte(`{"phase":"prepare","organization_name":"fixture-org","workspace_name":"import-root","query_run_id":"qry-fixture","selections":[{"candidate_id":"candidate-449c792d2631e92f1dd4026529d4ce07f9256f1a3e0bb1a95c05f2432b32a40a","managed_type":"aws_iam_role"}]}`), &input))
	return input
}

func importExecutionFixture(t *testing.T) (*importBackendTest, *bool, *bool) {
	t.Helper()
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	uploaded, finished := new(bool), new(bool)
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/workspaces/ws-fixture/configuration-versions":
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"speculative":true`)
			assert.Contains(t, string(body), `"auto-queue-runs":false`)
			assert.NotContains(t, string(body), `"provisional":true`)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"data":{"id":"cv-import","type":"configuration-versions","attributes":{"status":"pending","speculative":true,"auto-queue-runs":false,"upload-url":%q}}}`, f.url+"/upload")
		case "PUT /upload":
			assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
			assert.Empty(t, r.Header.Get("Authorization"))
			*uploaded = true
			w.WriteHeader(http.StatusOK)
		case "GET /api/v2/workspaces/ws-fixture/configuration-versions":
			assert.Equal(t, "1", r.URL.Query().Get("page[number]"))
			_, _ = io.WriteString(w, `{"data":[{"id":"cv-import","type":"configuration-versions","attributes":{"speculative":true}}],"meta":{"pagination":{"current-page":1,"next-page":0,"total-count":1,"total-pages":1}}}`)
		case "GET /api/v2/configuration-versions/cv-import":
			status := "pending"
			if *uploaded {
				status = "uploaded"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"cv-import","type":"configuration-versions","attributes":{"status":%q,"speculative":true,"auto-queue-runs":false}}}`, status)
		case "POST /api/v2/runs":
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"plan-only":true`)
			assert.Contains(t, string(body), `"id":"cv-import"`)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":"pending","plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`)
		case "GET /api/v2/runs/run-import":
			status := "pending"
			if *finished {
				status = "planned_and_finished"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":%q,"plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`, status)
		case "GET /api/v2/plans/plan-import":
			status := "pending"
			if *finished {
				status = "finished"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"plan-import","type":"plans","attributes":{"status":%q}}}`, status)
		case "GET /api/v2/plans/plan-import/json-output":
			_, _ = io.WriteString(w, `{"format_version":"1.2","variables":{"password":{"value":"MUST-NOT-LEAK"}},"resource_changes":[{"address":"aws_iam_role.selected","mode":"managed","type":"aws_iam_role","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"MUST-NOT-LEAK"}}},{"address":"aws_iam_role.unrelated","mode":"managed","change":{"actions":["update"]}}],"output_changes":{"new":{}}}`)
		default:
			return false
		}
		return true
	}
	return f, uploaded, finished
}

func parseTestPage(s string) int { var page int; _, _ = fmt.Sscan(s, &page); return page }

func blankImportFixture(t *testing.T) (*importBackendTest, *bool, *bool) {
	t.Helper()
	f, uploaded, finished := importExecutionFixture(t)
	f.stateStatus = http.StatusNotFound
	for _, path := range []string{"/api/v2/organizations/fixture-org/workspaces/import-root", "/api/v2/workspaces/ws-fixture"} {
		// Update structured fixture data, independent of whitespace or field order.
		var document map[string]any
		require.NoError(t, json.Unmarshal(f.responses[path], &document))
		rel := document["data"].(map[string]any)["relationships"].(map[string]any)
		rel["current-configuration-version"] = map[string]any{"data": nil}
		var err error
		f.responses[path], err = json.Marshal(document)
		require.NoError(t, err)
	}
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/v2/plans/plan-import/json-output":
				_, _ = io.WriteString(w, `{"format_version":"1.2","resource_changes":[]}`)
				return true
			case "/api/v2/runs/run-import/plan/json-schema":
				http.Redirect(w, r, f.url+"/schema-download?signed=fixture", http.StatusTemporaryRedirect)
				return true
			}
		}
		return original(w, r)
	}
	return f, uploaded, finished
}

// prepareFromAPIs runs the shared preparation internals, reading the QueryRun itself.
func prepareFromAPIs(ctx context.Context, c *tfe.Client, input importPrepareInput) importPreparation {
	return prepareImportWithDiscovery(ctx, c, input, nil)
}
