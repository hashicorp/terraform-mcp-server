// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const policyQueryLog = "\x02{\"type\":\"list_resource_found\",\"list_resource_found\":{\"address\":\"list.aws_instance.example\",\"display_name\":\"i-secret\",\"identity\":{\"id\":\"i-secret\"},\"resource_type\":\"aws_instance\"}}\n{\"type\":\"diagnostic\",\"diagnostic\":{\"severity\":\"error\",\"summary\":\"Query failed\",\"detail\":\"Provider returned an error.\"}}\n{\"type\":\"list_complete\",\"list_complete\":{\"address\":\"list.aws_instance.example\",\"resource_type\":\"aws_instance\",\"total\":1}}\n\x03"

// policyBackend models a backend whose token is authorized for organizations org-a and org-b.
type policyBackend struct {
	*httptest.Server
	mu        sync.Mutex
	paths     map[string]int
	queryRuns map[string]policyQueryRun
}

type policyQueryRun struct {
	status      string
	workspaceID string
}

func newPolicyBackend(t *testing.T) *policyBackend {
	t.Helper()
	b := &policyBackend{
		paths: map[string]int{},
		queryRuns: map[string]policyQueryRun{
			"qry-a-running":  {"running", "ws-a"},
			"qry-a-finished": {"finished", "ws-a"},
			"qry-a-errored":  {"errored", "ws-a"},
			"qry-b-pending":  {"pending", "ws-b"},
			"qry-b-running":  {"running", "ws-b"},
			"qry-b-finished": {"finished", "ws-b"},
			"qry-b-errored":  {"errored", "ws-b"},
			"qry-b-canceled": {"canceled", "ws-b"},
			"qry-no-owner":   {"finished", ""},
			"qry-ws-denied":  {"finished", "ws-denied"},
			"qry-no-org":     {"finished", "ws-no-org"},
		},
	}
	b.Server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.Close)
	return b
}

func (b *policyBackend) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v2/ping" {
		w.WriteHeader(http.StatusOK)
		return
	}
	b.mu.Lock()
	b.paths[r.URL.Path]++
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/vnd.api+json")
	switch {
	case r.URL.Path == "/logs":
		logBytes := []byte(policyQueryLog)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if offset < len(logBytes) {
			_, _ = w.Write(logBytes[offset:min(offset+limit, len(logBytes))])
		}
	case strings.HasPrefix(r.URL.Path, "/api/v2/queries/"):
		queryRun, ok := b.queryRuns[strings.TrimPrefix(r.URL.Path, "/api/v2/queries/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		relationships := ""
		if queryRun.workspaceID != "" {
			relationships = fmt.Sprintf(`,"relationships":{"workspace":{"data":{"type":"workspaces","id":%q}}}`, queryRun.workspaceID)
		}
		_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":%q,"attributes":{"status":%q,"terraform-version":"1.14.0","generate-config-out":false,"log-read-url":%q}%s}}`,
			strings.TrimPrefix(r.URL.Path, "/api/v2/queries/"), queryRun.status, b.URL+"/logs", relationships)
	case r.URL.Path == "/api/v2/workspaces/ws-denied":
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"status":"403","title":"forbidden"}]}`))
	case r.URL.Path == "/api/v2/workspaces/ws-no-org":
		_, _ = w.Write([]byte(`{"data":{"type":"workspaces","id":"ws-no-org","attributes":{"name":"w"}}}`))
	case strings.HasPrefix(r.URL.Path, "/api/v2/workspaces/ws-"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v2/workspaces/")
		_, _ = fmt.Fprintf(w, `{"data":{"type":"workspaces","id":%q,"attributes":{"name":"w"},"relationships":{"organization":{"data":{"type":"organizations","id":"org-%s"}}}}}`,
			id, strings.TrimPrefix(id, "ws-"))
	case strings.HasPrefix(r.URL.Path, "/api/v2/organizations/"):
		// Named-org workspace read; fail after the request is counted.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"status":"404","title":"not found"}]}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (b *policyBackend) count(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.paths[path]
}

func (b *policyBackend) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := 0
	for _, n := range b.paths {
		total += n
	}
	return total
}

func allowlistedContext(t *testing.T, b *policyBackend, allowlist ...string) context.Context {
	t.Helper()
	ctx := queryStatusHandlerContext(t, b.Server)
	if allowlist == nil {
		return ctx
	}
	return client.WithOrganizationAllowlist(ctx, client.BuildAllowedOrganizationsMap(allowlist))
}

func toolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}}
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	return text.Text
}

func TestGetQueryStatusEnforcesOrganizationAllowlist(t *testing.T) {
	for _, id := range []string{"qry-b-pending", "qry-b-running", "qry-b-finished", "qry-b-errored", "qry-b-canceled"} {
		t.Run("denies "+id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": id}), silentLogger(), testQueryStatusPollConfig())

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Nil(t, result.StructuredContent)
			text := resultText(t, result)
			assert.Contains(t, text, "not allowed by this server")
			assert.NotContains(t, text, `"status"`)
			assert.NotContains(t, text, "terminal")
			assert.Equal(t, 1, b.count("/api/v2/queries/"+id), "only the authorization read may occur; no polling")
			assert.Zero(t, b.count("/logs"))
		})
	}

	for id, want := range map[string]struct {
		status   string
		terminal bool
	}{
		"qry-a-running":  {"running", false},
		"qry-a-finished": {"finished", true},
		"qry-a-errored":  {"errored", true},
	} {
		t.Run("allows "+id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": id}), silentLogger(), testQueryStatusPollConfig())

			require.NoError(t, err)
			require.False(t, result.IsError, resultText(t, result))
			structured, ok := result.StructuredContent.(*queryStatusResponse)
			require.True(t, ok)
			assert.Equal(t, id, structured.ID)
			assert.EqualValues(t, want.status, structured.Status)
			assert.Equal(t, want.terminal, structured.Terminal)
			if want.terminal {
				assert.Zero(t, structured.RetryAfterSeconds)
			} else {
				assert.Equal(t, 5, structured.RetryAfterSeconds)
			}
		})
	}
}

func TestGetQueryStatusFailsClosedWhenOwnershipUnknown(t *testing.T) {
	for _, id := range []string{"qry-no-owner", "qry-no-org", "qry-ws-denied", "qry-missing"} {
		t.Run(id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": id}), silentLogger(), testQueryStatusPollConfig())

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Nil(t, result.StructuredContent)
			assert.Zero(t, b.count("/logs"))
		})
	}
}

func TestGetQueryStatusWithoutAllowlistKeepsBackendAuthorization(t *testing.T) {
	b := newPolicyBackend(t)
	ctx := allowlistedContext(t, b)

	result, err := getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": "qry-b-finished"}), silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	require.False(t, result.IsError, resultText(t, result))
	assert.Zero(t, b.count("/api/v2/workspaces/ws-b"), "no ownership lookup without an allowlist")

	result, err = getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": "qry-missing"}), silentLogger(), testQueryStatusPollConfig())
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "failed to get query run")
}

func TestGetQueryStatusAuthorizationHonorsCallerCancellation(t *testing.T) {
	b := newPolicyBackend(t)
	ctx, cancel := context.WithCancel(allowlistedContext(t, b, "org-a"))
	cancel()

	result, err := getQueryStatusHandlerWithConfig(ctx, toolRequest("get_query_status", map[string]any{"query_run_id": "qry-a-running"}), silentLogger(), testQueryStatusPollConfig())

	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, result.StructuredContent)
}

func TestGetQuerySummaryEnforcesOrganizationAllowlist(t *testing.T) {
	for _, id := range []string{"qry-b-finished", "qry-b-errored", "qry-b-running", "qry-b-canceled"} {
		t.Run("denies "+id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQuerySummaryHandler(ctx, toolRequest("get_query_summary", map[string]any{"query_run_id": id}), silentLogger())

			require.NoError(t, err)
			assert.True(t, result.IsError)
			text := resultText(t, result)
			assert.Contains(t, text, "not allowed by this server")
			assert.NotContains(t, text, "i-secret")
			assert.NotContains(t, text, "Query failed")
			assert.Zero(t, b.count("/logs"), "log must not be read")
		})
	}

	for _, id := range []string{"qry-a-finished", "qry-a-errored"} {
		t.Run("allows "+id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQuerySummaryHandler(ctx, toolRequest("get_query_summary", map[string]any{"query_run_id": id}), silentLogger())

			require.NoError(t, err)
			require.False(t, result.IsError, resultText(t, result))
			assert.JSONEq(t, `{"resources_discovered":1,"resources":[{"address":"list.aws_instance.example","display_name":"i-secret","identity":{"id":"i-secret"},"resource_type":"aws_instance"}],"list_completions":[{"address":"list.aws_instance.example","resource_type":"aws_instance","total":1}],"diagnostics":[{"severity":"error","summary":"Query failed","detail":"Provider returned an error."}]}`, resultText(t, result))
			assert.Positive(t, b.count("/logs"))
		})
	}
}

func TestGetQuerySummaryFailsClosedWhenOwnershipUnknown(t *testing.T) {
	for _, id := range []string{"qry-no-owner", "qry-no-org", "qry-ws-denied", "qry-missing"} {
		t.Run(id, func(t *testing.T) {
			b := newPolicyBackend(t)
			ctx := allowlistedContext(t, b, "org-a")

			result, err := getQuerySummaryHandler(ctx, toolRequest("get_query_summary", map[string]any{"query_run_id": id}), silentLogger())

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.NotContains(t, resultText(t, result), "i-secret")
			assert.Zero(t, b.count("/logs"))
		})
	}
}

func TestGetQuerySummaryWithoutAllowlistKeepsBackendAuthorization(t *testing.T) {
	b := newPolicyBackend(t)
	ctx := allowlistedContext(t, b)

	result, err := getQuerySummaryHandler(ctx, toolRequest("get_query_summary", map[string]any{"query_run_id": "qry-b-errored"}), silentLogger())

	require.NoError(t, err)
	require.False(t, result.IsError, resultText(t, result))
	assert.Contains(t, resultText(t, result), "Query failed")
	assert.Zero(t, b.count("/api/v2/workspaces/ws-b"))
}

func TestSearchNamedOrganizationCallsEnforceAllowlist(t *testing.T) {
	searchHandlers := map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"provider_list_schema_list": func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return providerListSchemaListHandler(ctx, r, silentLogger())
		},
		"execute_query": func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return executeQueryHandler(ctx, r, silentLogger())
		},
	}
	queryConfiguration := `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance"}}]}]}`

	for toolName, handler := range searchHandlers {
		call := func(b *policyBackend, args map[string]any) *mcp.CallToolResult {
			args["workspace_name"] = "w"
			args["query_configuration"] = queryConfiguration
			middleware := client.OrganizationAllowlistToolMiddleware([]string{"org-a"}, silentLogger())
			result, err := middleware(handler)(queryStatusHandlerContext(t, b.Server), toolRequest(toolName, args))
			require.NoError(t, err)
			return result
		}

		t.Run(toolName+" denies org-b", func(t *testing.T) {
			b := newPolicyBackend(t)
			for _, name := range []string{"org-b", " ORG-B "} {
				result := call(b, map[string]any{"organization_name": name})
				assert.True(t, result.IsError)
				assert.Contains(t, resultText(t, result), "not allowed by this server")
			}
			assert.Zero(t, b.total(), "backend must not be contacted for a disallowed organization")
		})

		t.Run(toolName+" denies conflicting aliases", func(t *testing.T) {
			b := newPolicyBackend(t)
			result := call(b, map[string]any{"organization_name": "org-b", "terraform_org_name": "org-a"})
			assert.True(t, result.IsError)
			assert.Zero(t, b.total())
		})

		t.Run(toolName+" denies non-string organization", func(t *testing.T) {
			b := newPolicyBackend(t)
			result := call(b, map[string]any{"organization_name": 7})
			assert.True(t, result.IsError)
			assert.Zero(t, b.total())
		})

		t.Run(toolName+" reaches backend for org-a", func(t *testing.T) {
			b := newPolicyBackend(t)
			result := call(b, map[string]any{"organization_name": "Org-A"})
			assert.NotContains(t, resultText(t, result), "not allowed by this server")
			assert.Equal(t, 1, b.count("/api/v2/organizations/Org-A/workspaces/w"))
		})
	}
}

func TestQueryStatusWaitBudgetUnchanged(t *testing.T) {
	assert.Equal(t, "40s", queryStatusWaitBudget.String())
	assert.Equal(t, "3s", queryStatusPollInterval.String())
}
