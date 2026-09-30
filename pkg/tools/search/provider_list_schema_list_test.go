// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentLogger() *log.Logger {
	l := log.New()
	l.SetLevel(log.PanicLevel)
	return l
}

func newProviderListTFEClient(t *testing.T, server *httptest.Server) *tfe.Client {
	t.Helper()
	tfeClient, err := tfe.NewClient(&tfe.Config{
		Address:    server.URL,
		Token:      "test-token",
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	return tfeClient
}

// ── helpers ───────────────────────────────────────────────────────────────────

// providerListServer spins up a test HTTP server that serves a fixed providers
// list at /api/v2/search/provider-versions and a fixed schema at
// /api/v2/search/provider-versions/:ns/:name/:version.
type providerListServer struct {
	server    *httptest.Server
	providers []providerEntry
	schemas   map[string]json.RawMessage // key: "ns/name/version"
}

type providerEntry struct {
	Namespace string
	Name      string
	Version   string
}

func newProviderListServer(providers []providerEntry, schemas map[string]json.RawMessage) *providerListServer {
	pls := &providerListServer{
		providers: providers,
		schemas:   schemas,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", pls.handler)
	pls.server = httptest.NewServer(mux)
	return pls
}

func (s *providerListServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.api+json")

	// Validate auth header
	if r.Header.Get("Authorization") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// /api/v2/search/provider-versions → list
	if r.URL.Path == "/api/v2/search/provider-versions" {
		type item struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
				Version   string `json:"version"`
			} `json:"attributes"`
		}
		var items []item
		for _, p := range s.providers {
			it := item{
				ID:   p.Namespace + "/" + p.Name + "/" + p.Version,
				Type: "provider-versions",
			}
			it.Attributes.Namespace = p.Namespace
			it.Attributes.Name = p.Name
			it.Attributes.Version = p.Version
			items = append(items, it)
		}
		resp := map[string]any{"data": items}
		if items == nil {
			resp["data"] = []any{}
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// /api/v2/search/provider-versions/:ns/:name/:version → schema
	// Strip leading /api/v2/search/provider-versions/
	const prefix = "/api/v2/search/provider-versions/"
	if len(r.URL.Path) > len(prefix) {
		key := r.URL.Path[len(prefix):]
		schema, ok := s.schemas[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		type attrs struct {
			Namespace           string          `json:"namespace"`
			Name                string          `json:"name"`
			Version             string          `json:"version"`
			ListResourceSchemas json.RawMessage `json:"list-resource-schemas"`
		}
		type data struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes attrs  `json:"attributes"`
		}
		parts := splitKey(key)
		resp := map[string]any{
			"data": data{
				ID:   key,
				Type: "provider-versions",
				Attributes: attrs{
					Namespace:           parts[0],
					Name:                parts[1],
					Version:             parts[2],
					ListResourceSchemas: schema,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func splitKey(key string) [3]string {
	// key format: "namespace/name/version"
	var parts [3]string
	idx := 0
	start := 0
	for i, ch := range key {
		if ch == '/' && idx < 2 {
			parts[idx] = key[start:i]
			idx++
			start = i + 1
		}
	}
	parts[idx] = key[start:]
	return parts
}

// ── listSupportedProviders ────────────────────────────────────────────────────

func TestListSupportedProviders_ReturnsList(t *testing.T) {
	providers := []providerEntry{
		{Namespace: "hashicorp", Name: "aws", Version: "5.0.0"},
		{Namespace: "hashicorp", Name: "google", Version: "4.0.0"},
	}
	s := newProviderListServer(providers, nil)
	defer s.server.Close()

	result, err := listSupportedProviders(context.Background(), newProviderListTFEClient(t, s.server), "", silentLogger())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	tc, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected TextContent")
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))

	supported := out["supported_providers"].([]any)
	assert.Len(t, supported, 2)
	assert.Equal(t, "hashicorp", supported[0].(map[string]any)["namespace"])
	assert.Equal(t, "aws", supported[0].(map[string]any)["name"])
}

func TestListSupportedProviders_EmptyList(t *testing.T) {
	s := newProviderListServer(nil, nil)
	defer s.server.Close()

	result, err := listSupportedProviders(context.Background(), newProviderListTFEClient(t, s.server), "", silentLogger())
	require.NoError(t, err)
	// Empty list → tool error result
	assert.True(t, result.IsError)
}

func TestListSupportedProviders_WithOrgFilter(t *testing.T) {
	var capturedOrg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOrg = r.URL.Query().Get("filter[organization][name]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]any{
					"id":   "hashicorp/aws/5.0.0",
					"type": "provider-versions",
					"attributes": map[string]any{
						"namespace": "hashicorp",
						"name":      "aws",
						"version":   "5.0.0",
					},
				},
			},
		})
	}))
	defer srv.Close()

	_, err := listSupportedProviders(context.Background(), newProviderListTFEClient(t, srv), "example org&group=test", silentLogger())
	require.NoError(t, err)
	assert.Equal(t, "example org&group=test", capturedOrg)
}

// ── discoverProviderVersion ───────────────────────────────────────────────────

func TestDiscoverProviderVersion_Found(t *testing.T) {
	providers := []providerEntry{
		{Namespace: "hashicorp", Name: "aws", Version: "5.1.0"},
		{Namespace: "hashicorp", Name: "google", Version: "4.2.0"},
	}
	s := newProviderListServer(providers, nil)
	defer s.server.Close()

	version, err := discoverProviderVersion(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws")
	require.NoError(t, err)
	assert.Equal(t, "5.1.0", version)
}
func TestDiscoverProviderVersion_WithOrgFilter(t *testing.T) {
	var capturedOrg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOrg = r.URL.Query().Get("filter[organization][name]")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	_, _ = discoverProviderVersion(context.Background(), newProviderListTFEClient(t, srv), "example org&group=test", "example", "provider")
	assert.Equal(t, "example org&group=test", capturedOrg)
}

func TestDiscoverProviderVersion_CaseInsensitive(t *testing.T) {
	providers := []providerEntry{
		{Namespace: "HashiCorp", Name: "AWS", Version: "5.1.0"},
	}
	s := newProviderListServer(providers, nil)
	defer s.server.Close()

	version, err := discoverProviderVersion(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws")
	require.NoError(t, err)
	assert.Equal(t, "5.1.0", version)
}

func TestDiscoverProviderVersion_NotFound(t *testing.T) {
	providers := []providerEntry{
		{Namespace: "hashicorp", Name: "google", Version: "4.0.0"},
	}
	s := newProviderListServer(providers, nil)
	defer s.server.Close()

	_, err := discoverProviderVersion(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hashicorp/aws is not in the search-compatible catalog")
}

// ── fetchProviderSchema ───────────────────────────────────────────────────────

func TestFetchProviderSchema_Success(t *testing.T) {
	schema := json.RawMessage(`{"aws_instance":{"block":{"attributes":{"id":{"type":"string"}}}}}`)
	schemas := map[string]json.RawMessage{
		"hashicorp/aws/5.0.0": schema,
	}
	s := newProviderListServer(nil, schemas)
	defer s.server.Close()

	result, err := fetchProviderSchema(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws", "5.0.0", silentLogger())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	tc, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected TextContent")
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	assert.Equal(t, "hashicorp", out["namespace"])
	assert.Equal(t, "aws", out["name"])
	assert.Equal(t, "5.0.0", out["version"])
	assert.NotNil(t, out["list_resource_schemas"])
	assert.Contains(t, out["note"].(string), "generate_query_configuration")
	assert.Contains(t, out["note"].(string), "exact resource type keys")
}

func TestFetchProviderSchema_NotInCatalog(t *testing.T) {
	s := newProviderListServer(nil, map[string]json.RawMessage{})
	defer s.server.Close()

	result, err := fetchProviderSchema(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws", "5.0.0", silentLogger())
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestFetchProviderSchema_NilListResourceSchemas(t *testing.T) {
	// Provider exists but has null list_resource_schemas
	schemas := map[string]json.RawMessage{
		"hashicorp/aws/5.0.0": nil,
	}
	s := newProviderListServer(nil, schemas)
	defer s.server.Close()

	result, err := fetchProviderSchema(context.Background(), newProviderListTFEClient(t, s.server), "", "hashicorp", "aws", "5.0.0", silentLogger())
	require.NoError(t, err)
	assert.True(t, result.IsError)
	tc, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected TextContent")
	assert.Contains(t, tc.Text, "no list_resource_schemas")
}

func TestFetchProviderSchema_WithOrgFilter(t *testing.T) {
	var capturedOrg string
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOrg = r.URL.Query().Get("filter[organization][name]")
		capturedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/vnd.api+json")
		schema := json.RawMessage(`{"aws_instance":{}}`)
		type attrs struct {
			Namespace           string          `json:"namespace"`
			Name                string          `json:"name"`
			Version             string          `json:"version"`
			ListResourceSchemas json.RawMessage `json:"list-resource-schemas"`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":   "hashicorp/aws/5.0.0",
				"type": "provider-versions",
				"attributes": attrs{
					Namespace:           "hashicorp",
					Name:                "aws",
					Version:             "5.0.0",
					ListResourceSchemas: schema,
				},
			},
		})
	}))
	defer srv.Close()

	_, _ = fetchProviderSchema(context.Background(), newProviderListTFEClient(t, srv), "example org&group=test", "example/namespace", "test provider", "1.0/test", silentLogger())
	assert.Equal(t, "example org&group=test", capturedOrg)
	assert.Equal(t, "/api/v2/search/provider-versions/example%2Fnamespace/test%20provider/1.0%2Ftest", capturedPath)
}

// ── tool definition ───────────────────────────────────────────────────────────

func TestProviderListSchemaList_ToolDefinition(t *testing.T) {
	logger := silentLogger()
	tool := ProviderListSchemaList(logger)

	assert.Equal(t, "provider_list_schema_list", tool.Tool.Name)
	assert.NotEmpty(t, tool.Tool.Description)
	assert.NotNil(t, tool.Handler)

	// Annotations
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Tool.Annotations.OpenWorldHint)

	// Parameters
	props := tool.Tool.InputSchema.Properties
	assert.Contains(t, props, "provider_namespace")
	assert.Contains(t, props, "provider_name")
	assert.NotContains(t, props, "provider_version")
	assert.Contains(t, props, "organization_name")
	assert.Contains(t, props, "workspace_name")
	assert.ElementsMatch(t, []string{"organization_name", "workspace_name"}, tool.Tool.InputSchema.Required)
	assert.Contains(t, tool.Tool.Description, "Do not attempt an unscoped request first")
	assert.Contains(t, tool.Tool.Description, "always read from the provider catalog response")
}

func TestProviderListSchemaList_RejectsMissingScopeBeforeRequest(t *testing.T) {
	result, err := providerListSchemaListHandler(context.Background(), mcp.CallToolRequest{}, silentLogger())

	require.NoError(t, err)
	require.True(t, result.IsError)
	tc, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected TextContent")
	assert.Contains(t, tc.Text, "organization_name and workspace_name are required")
	assert.Contains(t, tc.Text, "ask the user")
}

// ── toolErrorf ────────────────────────────────────────────────────────────────

func TestToolErrorf(t *testing.T) {
	result, err := toolErrorf(silentLogger(), "test_tool", "something went wrong: %v", "boom")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	tc, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "expected TextContent")
	assert.Equal(t, "something went wrong: boom", tc.Text)
}

func TestToolErrorf_NilLogger(t *testing.T) {
	result, err := toolErrorf(nil, "test_tool", "msg")
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ── URL building (splitKey helper) ───────────────────────────────────────────

func TestSplitKey(t *testing.T) {
	tests := []struct {
		key      string
		expected [3]string
	}{
		{"hashicorp/aws/5.0.0", [3]string{"hashicorp", "aws", "5.0.0"}},
		{"ns/name/1.2.3", [3]string{"ns", "name", "1.2.3"}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.expected, splitKey(tt.key))
		})
	}
}
