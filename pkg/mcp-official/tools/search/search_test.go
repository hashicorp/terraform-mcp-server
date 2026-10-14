// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchArgumentsAndValidation(t *testing.T) {
	require.Contains(t, GetQueryStatusTool().Description, "query_run_id returned by execute_query")
	require.NotContains(t, GetQueryStatusTool().Description, "latest-query-run")
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema, ok := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, statusSchema.Required, "terminal")
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")
	summarySchema, ok := GetQuerySummaryTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, summarySchema.Properties, "selectable")
	require.Contains(t, summarySchema.Properties, "lists")
	require.NotContains(t, summarySchema.Properties, "has_more")
	require.NotContains(t, summarySchema.Properties, "next_cursor")
	summaryInputSchema, err := jsonschema.For[QuerySummaryArguments](nil)
	require.NoError(t, err)
	require.NotContains(t, summaryInputSchema.Properties, "limit")
	require.NotContains(t, summaryInputSchema.Properties, "after")

	_, _, err = GetQueryStatusFunc(slog.Default())(context.Background(), nil, QueryRunArguments{QueryRunID: " "})
	require.ErrorContains(t, err, "missing required input: query_run_id")
	_, _, err = ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
		OrganizationName: "org", WorkspaceName: "ws", QueryConfiguration: `{`,
	})
	require.ErrorContains(t, err, "invalid query_configuration")
}

func TestOfficialSearchToolAnnotations(t *testing.T) {
	for _, tool := range []*mcp.Tool{GenerateQueryConfigurationTool(), ProviderListSchemaListTool(), GetQueryStatusTool(), GetQuerySummaryTool()} {
		require.True(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
	}
	require.False(t, ExecuteQueryTool().Annotations.ReadOnlyHint)
}
