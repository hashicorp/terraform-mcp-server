// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpanalytics

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestAnalyticsAreOnlyOnWithAProjectToken(t *testing.T) {
	for _, test := range []struct {
		name         string
		projectToken string
		host         string
		wantOn       bool
	}{
		{name: "nothing set"},
		{name: "a host without a token", host: "https://eu.i.posthog.com"},
		{name: "a project token", projectToken: "phc_test", wantOn: true},
		{name: "a project token and host", projectToken: "phc_test", host: "https://eu.i.posthog.com", wantOn: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MCP_POSTHOG_PROJECT_TOKEN", test.projectToken)
			t.Setenv("MCP_POSTHOG_HOST", test.host)

			analytics := Start(testLogger())
			defer analytics.Close()

			assert.Equal(t, test.wantOn, analytics != nil)
		})
	}
}

func TestCallsAreTaggedWithTheirOrganization(t *testing.T) {
	for _, test := range []struct {
		name       string
		orgNames   []string
		lookupErr  error
		wantTags   []map[string]any
		wantLookup []string
	}{
		{name: "a call without an organization", orgNames: []string{""}, wantTags: []map[string]any{nil}},
		{
			name:       "the organization is tagged by ID and normalized name, and read once",
			orgNames:   []string{" Acme-Corp", "acme-corp"},
			wantLookup: []string{"Acme-Corp"},
			wantTags:   []map[string]any{{"terraform_org_id": "org-acme123", "terraform_org_name": "acme-corp"}, {"terraform_org_id": "org-acme123", "terraform_org_name": "acme-corp"}},
		},
		{
			name:       "an organization that cannot be read is not tagged",
			orgNames:   []string{"acme-typo"},
			wantLookup: []string{"acme-typo"},
			lookupErr:  errors.New("not found"),
			wantTags:   []map[string]any{nil},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			posthogServer := &fakePostHog{}
			endpoint := httptest.NewServer(posthogServer)
			defer endpoint.Close()
			t.Setenv("MCP_POSTHOG_PROJECT_TOKEN", "phc_test")
			t.Setenv("MCP_POSTHOG_HOST", endpoint.URL)
			analytics := Start(testLogger())
			var lookedUp []string
			analytics.lookupOrgID = func(_ context.Context, _ *mcp.CallToolRequest, orgName string) (string, error) {
				lookedUp = append(lookedUp, orgName)
				return "org-acme123", test.lookupErr
			}
			session := connectToInstrumentedServer(t, analytics)
			defer session.Close()

			for _, orgName := range test.orgNames {
				_, err := session.CallTool(t.Context(), &mcp.CallToolParams{
					Name:      "get_workspace",
					Arguments: map[string]any{"workspace": "prod-network", "terraform_org_name": orgName},
				})
				require.NoError(t, err)
			}
			analytics.Close()

			var tags []map[string]any
			for _, event := range posthogServer.eventProperties(t, "$mcp_tool_call") {
				var tag map[string]any
				for _, key := range []string{"terraform_org_id", "terraform_org_name"} {
					if value, ok := event[key]; ok {
						if tag == nil {
							tag = map[string]any{}
						}
						tag[key] = value
					}
				}
				tags = append(tags, tag)
			}
			assert.Equal(t, test.wantTags, tags)
			assert.Equal(t, test.wantLookup, lookedUp)
		})
	}
}

func connectToInstrumentedServer(t *testing.T, analytics *Analytics) *mcp.ClientSession {
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_workspace"},
		func(context.Context, *mcp.CallToolRequest, struct {
			Workspace string `json:"workspace"`
			OrgName   string `json:"terraform_org_name,omitempty"`
		}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "workspace details"}}}, nil, nil
		})
	analytics.Instrument(srv)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil).Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	return session
}

func TestToolCallsReachPostHogWithoutArguments(t *testing.T) {
	posthogServer := &fakePostHog{}
	endpoint := httptest.NewServer(posthogServer)
	defer endpoint.Close()
	t.Setenv("MCP_POSTHOG_PROJECT_TOKEN", "phc_test")
	t.Setenv("MCP_POSTHOG_HOST", endpoint.URL)

	analytics := Start(testLogger())
	session := connectToInstrumentedServer(t, analytics)
	defer session.Close()

	listed, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	schema, err := json.Marshal(listed.Tools[0].InputSchema)
	require.NoError(t, err)
	var advertised struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(schema, &advertised))
	assert.Equal(t, []string{"context", "llm_model", "terraform_org_name", "workspace"}, slices.Sorted(maps.Keys(advertised.Properties)))

	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "get_workspace",
		Arguments: map[string]any{"workspace": "prod-network", "context": "Checking a workspace before a change"},
	})
	require.NoError(t, err)
	analytics.Close()

	event := posthogServer.eventProperties(t, "$mcp_tool_call")[0]
	assert.Equal(t, "terraform-mcp-server", event["$mcp_server_name"])
	assert.Equal(t, "get_workspace", event["$mcp_tool_name"])
	assert.Equal(t, "Checking a workspace before a change", event["$mcp_intent"])
	assert.NotContains(t, event, "$mcp_parameters")
	assert.NotContains(t, event, "$mcp_response")
}

type fakePostHog struct {
	mu     sync.Mutex
	events []map[string]any
}

func (f *fakePostHog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := io.Reader(r.Body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body = gz
	}
	var batch struct {
		Batch []map[string]any `json:"batch"`
	}
	if err := json.NewDecoder(body).Decode(&batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, batch.Batch...)
	_, _ = w.Write([]byte(`{"status":1}`))
}

func (f *fakePostHog) eventProperties(t *testing.T, name string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []map[string]any
	for _, event := range f.events {
		if event["event"] == name {
			found = append(found, event["properties"].(map[string]any))
		}
	}
	require.NotEmpty(t, found, "events received: %v", f.events)
	return found
}
