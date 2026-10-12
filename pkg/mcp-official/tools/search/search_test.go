// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"testing"

	oldmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestConvertResultPreservesStatusAndErrors(t *testing.T) {
	status := map[string]any{"query_run_id": "qr-1", "status": "finished", "terminal": true}
	old := oldmcp.NewToolResultStructured(status, "Query run finished\n\nstatus JSON")
	result, output, err := convertResult(old, nil)
	require.NoError(t, err)
	require.Nil(t, output)
	require.False(t, result.IsError)
	require.Equal(t, status, result.StructuredContent)
	require.Equal(t, "Query run finished\n\nstatus JSON", result.Content[0].(*mcp.TextContent).Text)

	old = oldmcp.NewToolResultError("read failed")
	result, output, err = convertResult(old, nil)
	require.NoError(t, err)
	require.Nil(t, output)
	require.True(t, result.IsError)
	require.Equal(t, "read failed", result.Content[0].(*mcp.TextContent).Text)
}

func TestOfficialSearchToolAnnotations(t *testing.T) {
	for _, tool := range []*mcp.Tool{GenerateQueryConfigurationTool(), ProviderListSchemaListTool(), GetQueryStatusTool(), GetQuerySummaryTool()} {
		require.True(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
	}
	require.False(t, ExecuteQueryTool().Annotations.ReadOnlyHint)
}
