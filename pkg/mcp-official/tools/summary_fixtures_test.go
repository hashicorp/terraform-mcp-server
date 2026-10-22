// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	legacymcp "github.com/mark3labs/mcp-go/mcp"
)

// Read-only fixtures deliberately have no destination, Run, Plan, preparation,
// or verification routes. The later import fixtures remain in the source tree.
type summaryTestSession struct{}

func (summaryTestSession) Initialize()                                               {}
func (summaryTestSession) Initialized() bool                                         { return true }
func (summaryTestSession) NotificationChannel() chan<- legacymcp.JSONRPCNotification { return nil }
func (summaryTestSession) SessionID() string                                         { return "" }

func summaryLogFixture(version string) string {
	return `{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"first","identity":{"bucket":"first"},"identity_version":` + version + `,"resource_object":{"password":"resource-object-secret-sentinel","tags":{"team":"search"}},"config":"resource \"aws_s3_bucket\" \"first\" { bucket = \"first\" }","import_config":"import { id = \"must-not-be-returned\" }"}}` + "\n" +
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":1}}` + "\n"
}

func summaryReadOnlyBackend(t *testing.T, status, organization, logBody, failPath string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	logReads := new(atomic.Int32)
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == failPath {
			http.Error(w, "backend-secret-sentinel", http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":%q,"generate-config-out":true,"log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, status, backend.URL+"/logs?X-Amz-Signature=signed-secret-sentinel")
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":%q}}}}}`, organization)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"HashiCorp","name":"AWS","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		case "/logs":
			logReads.Add(1)
			body := []byte("\x02Starting query\n" + `{"type":"provider_installation_start","provider":"aws"}` + "\n" + logBody + "\n\x03")
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(body) {
				_, _ = w.Write(body[offset:min(len(body), offset+limit)])
			}
		default:
			t.Errorf("unexpected backend access: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	return backend, logReads
}
