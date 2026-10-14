// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type searchProviderCatalog struct {
	SupportedProviders []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Version   string `json:"version"`
	} `json:"supported_providers"`
}

type searchProviderSchema struct {
	Namespace           string                     `json:"namespace"`
	Name                string                     `json:"name"`
	Version             string                     `json:"version"`
	ListResourceSchemas map[string]json.RawMessage `json:"list_resource_schemas"`
}

type searchQuerySummary struct {
	ResourcesDiscovered int `json:"resources_discovered"`
	Resources           []struct {
		Address      string         `json:"address"`
		DisplayName  string         `json:"display_name"`
		Identity     map[string]any `json:"identity"`
		ResourceType string         `json:"resource_type"`
	} `json:"resources"`
	ListCompletions []struct {
		ResourceType string `json:"resource_type"`
		Total        int    `json:"total"`
	} `json:"list_completions"`
	Diagnostics []struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
		Detail   string `json:"detail"`
	} `json:"diagnostics"`
}

type searchQueryStatus struct {
	QueryRunID        string `json:"query_run_id"`
	Status            string `json:"status"`
	Terminal          bool   `json:"terminal"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

func TestTerraformSearchWorkflow(t *testing.T) {
	requireTfOperations(t)
	workspaceName := strings.TrimSpace(os.Getenv("TF_SEARCH_WORKSPACE"))
	if tfeToken == "" || workspaceName == "" {
		t.Skip("supply TFE_TOKEN and TF_SEARCH_WORKSPACE for the live Search fixture")
	}

	s := newSearchTestingSession(t)
	defer s.Close()

	available, err := s.ListTools(t.Context(), nil)
	require.NoError(t, err)
	toolNames := make([]string, 0, len(available.Tools))
	for _, tool := range available.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	for _, name := range []string{"provider_list_schema_list", "generate_query_configuration", "execute_query", "get_query_status", "get_query_summary"} {
		require.Contains(t, toolNames, name, "start the server with the search toolset enabled")
	}

	listResult, listText := callSearchTool(t, s, "provider_list_schema_list", map[string]any{
		"organization_name": tfeOrgName,
		"workspace_name":    workspaceName,
	})
	require.False(t, listResult.IsError, "provider catalog request should succeed: %s", listText)

	var catalog searchProviderCatalog
	require.NoError(t, json.Unmarshal([]byte(listText), &catalog), "provider catalog should return valid JSON")
	require.NotEmpty(t, catalog.SupportedProviders, "organization should have at least one search-compatible provider")

	providerNamespace := searchFixtureValue("TF_SEARCH_PROVIDER_NAMESPACE", "hashicorp")
	providerName := searchFixtureValue("TF_SEARCH_PROVIDER_NAME", "aws")
	resourceType := searchFixtureValue("TF_SEARCH_RESOURCE_TYPE", "aws_instance")
	providerIndex := -1
	for i, candidate := range catalog.SupportedProviders {
		if candidate.Namespace == providerNamespace && candidate.Name == providerName {
			providerIndex = i
			break
		}
	}
	require.NotEqual(t, -1, providerIndex, "fixture provider %s/%s is no longer in the Search catalog; review the fixture, do not substitute an arbitrary provider", providerNamespace, providerName)
	provider := catalog.SupportedProviders[providerIndex]
	require.NotEmpty(t, provider.Version)
	schemaResult, schemaText := callSearchTool(t, s, "provider_list_schema_list", map[string]any{
		"organization_name":  tfeOrgName,
		"workspace_name":     workspaceName,
		"provider_namespace": provider.Namespace,
		"provider_name":      provider.Name,
	})
	require.False(t, schemaResult.IsError, "provider schema request should succeed: %s", schemaText)

	var schema searchProviderSchema
	require.NoError(t, json.Unmarshal([]byte(schemaText), &schema), "provider schema should return valid JSON")
	require.NotEmpty(t, schema.ListResourceSchemas, "provider should expose at least one list resource schema")
	assert.Equal(t, provider.Namespace, schema.Namespace)
	assert.Equal(t, provider.Name, schema.Name)
	assert.Equal(t, provider.Version, schema.Version)

	require.Contains(t, schema.ListResourceSchemas, resourceType, "fixture list resource is not supported by catalog version %s; review the fixture", provider.Version)
	t.Logf("Search fixture: organization=%s workspace=%s provider=%s/%s@%s list_resource=%s", tfeOrgName, workspaceName, provider.Namespace, provider.Name, provider.Version, resourceType)

	generateResult, guideText := callSearchTool(t, s, "generate_query_configuration", map[string]any{
		"list_resource_schemas": schemaText,
		"provider_namespace":    schema.Namespace,
		"provider_name":         schema.Name,
		"provider_version":      schema.Version,
		"resource_types":        resourceType,
	})
	require.False(t, generateResult.IsError, "query configuration generation should succeed: %s", guideText)
	queryConfiguration := extractSearchExamplePayload(t, guideText)
	assert.Equal(t, resourceType, gjson.Get(queryConfiguration, "no_code_query_providers.0.no_code_query_resources.0.body.resource_type").String())
	assert.Equal(t, provider.Version, gjson.Get(queryConfiguration, "no_code_query_providers.0.version").String())
	// This fixture deliberately needs no required list configuration. Never send
	// generated placeholder values or secrets as query attributes.
	require.Empty(t, gjson.Get(queryConfiguration, "no_code_query_providers.0.no_code_query_resources.0.body.attributes").Array(), "fixture now needs required list attributes; review the schema and choose a credential-backed fixture without required attributes")
	require.False(t, gjson.Get(queryConfiguration, "generate_config_out").Bool())

	t.Run("invalid_configuration", func(t *testing.T) {
		result, text := callSearchTool(t, s, "execute_query", map[string]any{
			"organization_name": tfeOrgName, "workspace_name": workspaceName, "query_configuration": "{",
		})
		require.True(t, result.IsError)
		assert.Contains(t, text, "invalid query_configuration")
		assert.Contains(t, text, "not valid JSON")
	})
	t.Run("unsupported_provider", func(t *testing.T) {
		result, text := callSearchTool(t, s, "provider_list_schema_list", map[string]any{
			"organization_name": tfeOrgName, "workspace_name": workspaceName,
			"provider_namespace": "mcp-search-test-unsupported", "provider_name": "nonexistent",
		})
		require.True(t, result.IsError)
		assert.Contains(t, text, "not in the search-compatible catalog")
		assert.Contains(t, text, "provider_list_schema_list")
	})
	t.Run("unsupported_list_resource", func(t *testing.T) {
		result, text := callSearchTool(t, s, "generate_query_configuration", map[string]any{
			"list_resource_schemas": schemaText, "provider_namespace": schema.Namespace,
			"provider_name": schema.Name, "provider_version": schema.Version,
			"resource_types": "mcp_search_test_nonexistent_resource",
		})
		require.True(t, result.IsError)
		assert.Contains(t, text, "requested resource types")
		assert.Contains(t, text, "none of")
		assert.Contains(t, text, "schema")
	})
	t.Run("inaccessible_workspace", func(t *testing.T) {
		missingWorkspace := randomName("search-missing-")
		result, text := callSearchTool(t, s, "provider_list_schema_list", map[string]any{
			"organization_name": tfeOrgName, "workspace_name": missingWorkspace,
		})
		require.True(t, result.IsError)
		assert.Contains(t, text, "failed to read workspace")
		assert.Contains(t, text, missingWorkspace)
		assert.Contains(t, text, tfeOrgName)
	})

	executeResult, executeText := callSearchTool(t, s, "execute_query", map[string]any{
		"organization_name":   tfeOrgName,
		"workspace_name":      workspaceName,
		"query_configuration": queryConfiguration,
	})
	require.False(t, executeResult.IsError, "query execution should be accepted: %s", executeText)
	queryRunID := gjson.Get(executeText, "data.relationships.latest-query-run.data.id").String()
	require.NotEmpty(t, queryRunID, "execute_query should return a query run ID")
	t.Logf("Search query run: %s (retained in fixture workspace for diagnosis)", queryRunID)

	status := waitForSearchQuery(t, s, queryRunID)

	summaryResult, summaryText := callSearchTool(t, s, "get_query_summary", map[string]any{
		"query_run_id": queryRunID,
	})
	require.False(t, summaryResult.IsError, "query summary should be available: %s", summaryText)

	var summary searchQuerySummary
	require.NoError(t, json.Unmarshal([]byte(summaryText), &summary), "query summary should return valid JSON")
	for _, field := range []string{"resources_discovered", "resources", "list_completions", "diagnostics"} {
		require.True(t, gjson.Get(summaryText, field).Exists(), "summary missing %s", field)
	}
	require.NotNil(t, summary.Resources)
	require.NotNil(t, summary.ListCompletions)
	require.NotNil(t, summary.Diagnostics)
	// Fetch the summary even for failed/canceled runs so diagnostics are available.
	require.Equal(t, "finished", status.Status, "fixture query did not finish successfully; diagnostics: %+v", summary.Diagnostics)
	require.Len(t, summary.ListCompletions, 1, "query should report a completed resource search: %s", summaryText)
	assert.GreaterOrEqual(t, summary.ResourcesDiscovered, 0)
	assert.LessOrEqual(t, summary.ResourcesDiscovered, 100, "generated fixture limits discovery to 100 records")
	assert.Equal(t, resourceType, summary.ListCompletions[0].ResourceType)
	assert.Equal(t, summary.ResourcesDiscovered, summary.ListCompletions[0].Total)
	assert.Len(t, summary.Resources, summary.ResourcesDiscovered)
	for _, resource := range summary.Resources {
		assert.Equal(t, resourceType, resource.ResourceType)
		assert.NotEmpty(t, resource.Address)
		assert.NotEmpty(t, resource.Identity, "discovered records must have a usable identity")
	}
	for _, diagnostic := range summary.Diagnostics {
		assert.NotEmpty(t, diagnostic.Severity)
		assert.NotEmpty(t, diagnostic.Summary)
		assert.NotEqual(t, "error", diagnostic.Severity, "%s: %s", diagnostic.Summary, diagnostic.Detail)
	}
}

func searchFixtureValue(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func newSearchTestingSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), toolCallTimeout)
	defer cancel()
	session, err := testingClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: mcpEndpoint,
		HTTPClient: &http.Client{
			// Each status call has a bounded server-side polling budget.
			Timeout:   toolCallTimeout,
			Transport: &authTransport{token: tfeToken, roundtripper: http.DefaultTransport},
		},
	}, nil)
	require.NoError(t, err, "connect to the running Search-enabled MCP server")
	return session
}

// Do not log arguments: schema/configuration values and cloud identities may be
// sensitive. Only surface tool errors; credentials live in the HTTP transport.
func callSearchTool(t *testing.T, s *mcp.ClientSession, name string, arguments map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), toolCallTimeout)
	defer cancel()
	result, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err, "MCP call %s failed", name)
	require.NotNil(t, result)
	return result, getTextContent(result)
}

func waitForSearchQuery(t *testing.T, s *mcp.ClientSession, queryRunID string) searchQueryStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	var status searchQueryStatus
	for {
		callCtx, cancelCall := context.WithTimeout(ctx, toolCallTimeout)
		result, err := s.CallTool(callCtx, &mcp.CallToolParams{
			Name: "get_query_status", Arguments: map[string]any{"query_run_id": queryRunID},
		})
		cancelCall()
		require.NoError(t, err, "poll query %s; last status: %s", queryRunID, status.Status)
		require.NotNil(t, result)
		text := getTextContent(result)
		require.False(t, result.IsError, "query status request failed: %s", text)
		status, err = parseSearchQueryStatus(text, queryRunID)
		require.NoError(t, err)
		if status.Terminal {
			return status
		}
		timer := time.NewTimer(time.Duration(status.RetryAfterSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("timed out waiting for query %s; last status: %s", queryRunID, status.Status)
		case <-timer.C:
		}
	}
}

// Both SDKs expose the same JSON fallback after the human-readable guidance.
func parseSearchQueryStatus(text, queryRunID string) (searchQueryStatus, error) {
	var status searchQueryStatus
	_, payload, found := strings.Cut(text, "\n\n")
	if !found {
		return status, fmt.Errorf("status response must contain its JSON details")
	}
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		return status, fmt.Errorf("invalid status JSON: %w", err)
	}
	if status.QueryRunID != queryRunID {
		return status, fmt.Errorf("status query_run_id %q does not match %q", status.QueryRunID, queryRunID)
	}
	terminal := status.Status == "finished" || status.Status == "errored" || status.Status == "canceled"
	if status.Terminal != terminal {
		return status, fmt.Errorf("terminal flag does not match status %q", status.Status)
	}
	if !status.Terminal && (status.Status == "" || status.RetryAfterSeconds <= 0) {
		return status, fmt.Errorf("nonterminal status must include a status and positive retry_after_seconds")
	}
	return status, nil
}

func TestParseSearchQueryStatus(t *testing.T) {
	for _, test := range []struct {
		name, text, wantError string
		terminal              bool
	}{
		{"finished", "Done\n\n{\"query_run_id\":\"qr-test\",\"status\":\"finished\",\"terminal\":true}", "", true},
		{"errored", "Done\n\n{\"query_run_id\":\"qr-test\",\"status\":\"errored\",\"terminal\":true}", "", true},
		{"canceled", "Done\n\n{\"query_run_id\":\"qr-test\",\"status\":\"canceled\",\"terminal\":true}", "", true},
		{"running", "Retry\n\n{\"query_run_id\":\"qr-test\",\"status\":\"running\",\"terminal\":false,\"retry_after_seconds\":5}", "", false},
		{"missing JSON", "Done", "JSON details", false},
		{"malformed JSON", "Done\n\n{", "invalid status JSON", false},
		{"old ID field", "Done\n\n{\"id\":\"qr-test\",\"status\":\"finished\",\"terminal\":true}", "does not match", false},
		{"inconsistent terminal", "Done\n\n{\"query_run_id\":\"qr-test\",\"status\":\"running\",\"terminal\":true}", "terminal flag", false},
		{"missing retry", "Retry\n\n{\"query_run_id\":\"qr-test\",\"status\":\"running\",\"terminal\":false}", "positive retry_after_seconds", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, err := parseSearchQueryStatus(test.text, "qr-test")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.terminal, status.Terminal)
		})
	}
}

func extractSearchExamplePayload(t *testing.T, guide string) string {
	t.Helper()

	const heading = "## Example No-Code Query Payload"
	headingIndex := strings.Index(guide, heading)
	require.NotEqual(t, -1, headingIndex, "generated guide should contain an example payload section")

	section := guide[headingIndex+len(heading):]
	const fence = "```json\n"
	fenceIndex := strings.Index(section, fence)
	require.NotEqual(t, -1, fenceIndex, "example payload section should contain a JSON code block")

	payload := section[fenceIndex+len(fence):]
	endIndex := strings.Index(payload, "\n```")
	require.NotEqual(t, -1, endIndex, "example payload JSON code block should be closed")
	payload = payload[:endIndex]
	require.True(t, gjson.Valid(payload), "example payload should be valid JSON")
	return payload
}
