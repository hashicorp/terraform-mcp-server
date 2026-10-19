// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validExecuteQueryConfiguration = `{
  "generate_config_out": false,
  "no_code_query_providers": [{
    "namespace": "hashicorp",
    "name": "aws",
    "version": "6.33.0",
    "no_code_query_resources": [{
      "body": {
        "resource_type": "aws_instance",
        "limit": 25,
        "attributes": [{"attribute": "region", "value": "${var.region}"}]
      }
    }]
  }]
}`

func TestExecuteQueryDefinition(t *testing.T) {
	tool := ExecuteQuery(silentLogger())

	assert.Equal(t, "execute_query", tool.Tool.Name)
	assert.NotContains(t, tool.Tool.InputSchema.Required, "organization_name")
	assert.NotContains(t, tool.Tool.InputSchema.Required, "workspace_name")
	assert.Contains(t, tool.Tool.Description, "TF_MCP_QUERY_WORKSPACE")
	assert.NotContains(t, tool.Tool.InputSchema.Properties, "workspace_id")
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_configuration")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Tool.Annotations.OpenWorldHint)
	assert.Contains(t, tool.Tool.Description, "provider_list_schema_list")
	assert.Contains(t, tool.Tool.Description, "generate_query_configuration")
	assert.Contains(t, tool.Tool.Description, "get_query_status")
	assert.Contains(t, tool.Tool.Description, "exact list_resource_schemas key")
	assert.Contains(t, tool.Tool.Description, "ordinary managed-resource name")
	assert.Contains(t, tool.Tool.Description, "confirm the source organization/workspace")
	assert.Contains(t, tool.Tool.Description, "TF_MCP_QUERY_WORKSPACE")
	assert.Contains(t, tool.Tool.Description, "wait for")
	assert.Contains(t, tool.Tool.Description, "approval")
	assert.Contains(t, tool.Tool.Description, "obtain approval again")
	assert.Contains(t, tool.Tool.Description, "reconcile")
	assert.Contains(t, tool.Tool.Description, "do not blindly execute")
	assert.Contains(t, tool.Tool.OutputSchema.Required, "query_run_id")
}

func TestParseExecuteQueryConfiguration(t *testing.T) {
	configuration, err := parseExecuteQueryConfiguration(validExecuteQueryConfiguration)

	require.NoError(t, err)
	assert.Equal(t, "aws", configuration.Providers[0].Name)
	require.NotNil(t, configuration.GenerateConfigOut)
	assert.False(t, *configuration.GenerateConfigOut)
}

func TestParseExecuteQueryConfigurationValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "invalid JSON", payload: `{`, want: "not valid JSON"},
		{name: "no providers", payload: `{"no_code_query_providers":[]}`, want: "at least one"},
		{name: "missing provider name", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance"}}]}]}`, want: "name is required"},
		{name: "missing resource type", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{}}]}]}`, want: "body.resource_type"},
		{name: "zero limit", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","limit":0}}]}]}`, want: "positive integer"},
		{name: "fractional limit", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","limit":1.5}}]}]}`, want: "positive integer"},
		{name: "quoted limit", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","limit":"25"}}]}]}`, want: "positive integer"},
		{name: "blank attribute", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","attributes":[{"attribute":""}]}}]}]}`, want: "name is required"},
		{name: "missing attribute value", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","attributes":[{"attribute":"region"}]}}]}]}`, want: "value is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseExecuteQueryConfiguration(test.payload)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestSubmitExecuteQuery(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/search/no-code-query", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.api+json", r.Header.Get("Accept"))
		assert.Equal(t, "application/vnd.api+json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"type":"no-code-queries","id":"ncqry-test","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-test"}}}}}`))
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{
		Address:    server.URL,
		Token:      "test-token",
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	configuration, err := parseExecuteQueryConfiguration(validExecuteQueryConfiguration)
	require.NoError(t, err)

	response, err := submitExecuteQuery(context.Background(), tfeClient, "ws-test", configuration)

	require.NoError(t, err)
	assert.Equal(t, &ExecuteQueryResponse{QueryRunID: "qry-test"}, response)
	assert.Equal(t, "no-code-queries", received["data"].(map[string]any)["type"])
	attributes := received["data"].(map[string]any)["attributes"].(map[string]any)
	assert.Equal(t, "terraform-mcp", attributes["source"])
	assert.Equal(t, false, attributes["generate-config-out"])
	providers := attributes["no-code-query-providers"].([]any)
	assert.Equal(t, "aws", providers[0].(map[string]any)["name"])
	workspace := received["data"].(map[string]any)["relationships"].(map[string]any)["workspace"].(map[string]any)
	assert.Equal(t, "ws-test", workspace["data"].(map[string]any)["id"])
}

func TestSubmitExecuteQueryReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"Limit must be a positive integer"}]}`))
	}))
	defer server.Close()

	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "test-token", HTTPClient: server.Client()})
	require.NoError(t, err)
	configuration, err := parseExecuteQueryConfiguration(validExecuteQueryConfiguration)
	require.NoError(t, err)

	_, err = submitExecuteQuery(context.Background(), tfeClient, "ws-test", configuration)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Limit must be a positive integer")
}

func TestSubmitQueryProjectsIDAndDoesNotExposeBackendCapabilities(t *testing.T) {
	const secret = "https://archivist.example/signed?token=secret"
	for _, tc := range []struct {
		name, status, body, wantID string
		wantErr                    bool
	}{
		{"created", "201", `{"data":{"id":"ncqry-test","download_url":"` + secret + `","relationships":{"latest-query-run":{"data":{"id":"qry-test"}}}}}`, "qry-test", false},
		{"missing relationship", "201", `{"data":{"id":"ncqry-test","url":"` + secret + `"}}`, "", true},
		{"opaque ID", "201", `{"data":{"relationships":{"latest-query-run":{"data":{"id":"future.query:id"}}}}}`, "future.query:id", false},
		{"malformed body", "201", `{"data":`, "", true},
		{"large valid body", "201", `{"data":{"relationships":{"latest-query-run":{"data":{"id":"qry-test"}}}}}` + strings.Repeat(" ", 64<<10), "qry-test", false},
		{"rejected POST", "422", `{"errors":[{"detail":"` + secret + `"}]}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/ping" {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.Header().Set("Content-Type", "application/vnd.api+json")
				if r.URL.Path == "/api/v2/organizations/org/workspaces/ws" {
					_, _ = w.Write([]byte(`{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`))
					return
				}
				if tc.status == "422" {
					w.WriteHeader(http.StatusUnprocessableEntity)
				} else {
					w.WriteHeader(http.StatusCreated)
				}
				_, _ = fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(backend.Close)
			client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
			require.NoError(t, err)
			text, err := ExecuteQueryText(context.Background(), client, "org", "ws", validExecuteQueryConfiguration)
			if tc.wantErr {
				require.ErrorContains(t, err, "outcome unknown")
				assert.NotContains(t, err.Error(), secret)
				assert.Empty(t, text)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, fmt.Sprintf(`{"query_run_id":%q}`, tc.wantID), text)
			assert.False(t, strings.Contains(text, secret))
		})
	}
}

func TestSubmitQueryWorkspaceErrorDoesNotExposeBackendBody(t *testing.T) {
	const sentinel = "https://archivist.example/signed?secret=sentinel"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "/api/v2/organizations/org/workspaces/ws", r.URL.Path)
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"errors":[{"detail":"`+sentinel+`"}]}`)
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	_, err = SubmitQuery(context.Background(), client, "org", "ws", validExecuteQueryConfiguration)
	require.ErrorContains(t, err, "unable to read source workspace")
	assert.NotContains(t, err.Error(), sentinel)
}

func TestLegacyExecuteQueryStructuredProjection(t *testing.T) {
	const sentinel = "https://archivist.example/signed?secret=sentinel"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/organizations/org/workspaces/ws":
			_, _ = w.Write([]byte(`{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`))
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"data":{"url":"`+sentinel+`","relationships":{"latest-query-run":{"data":{"id":"qry-test"}}}}}`)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	result, err := executeQueryWithClient(context.Background(), client, "org", "ws", validExecuteQueryConfiguration, silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.JSONEq(t, `{"query_run_id":"qry-test"}`, text.Text)
	assert.Equal(t, "qry-test", result.StructuredContent.(*ExecuteQueryResponse).QueryRunID)
	assert.NotContains(t, fmt.Sprint(result), sentinel)
}
