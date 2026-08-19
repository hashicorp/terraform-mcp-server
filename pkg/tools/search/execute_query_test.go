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
	assert.Contains(t, tool.Tool.InputSchema.Required, "organization_name")
	assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
	assert.NotContains(t, tool.Tool.InputSchema.Properties, "workspace_id")
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_configuration")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Tool.Annotations.OpenWorldHint)
	assert.Contains(t, tool.Tool.Description, "get_query_status")
	assert.Contains(t, tool.Tool.Description, "Do not use curl")
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
		{name: "blank attribute", payload: `{"no_code_query_providers":[{"namespace":"hashicorp","name":"aws","version":"1.0.0","no_code_query_resources":[{"body":{"resource_type":"aws_instance","attributes":[{"attribute":""}]}}]}]}`, want: "name is required"},
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
	assert.JSONEq(t, `{"data":{"type":"no-code-queries","id":"ncqry-test","relationships":{"latest-query-run":{"data":{"type":"queries","id":"qry-test"}}}}}`, response)
	assert.Equal(t, "no-code-queries", received["data"].(map[string]any)["type"])
	attributes := received["data"].(map[string]any)["attributes"].(map[string]any)
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
