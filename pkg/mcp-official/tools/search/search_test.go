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
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema, ok := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, statusSchema.Required, "terminal")
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")

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
