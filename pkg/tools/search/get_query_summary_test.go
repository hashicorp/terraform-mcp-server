// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQuerySummaryDefinition(t *testing.T) {
	tool := GetQuerySummary(silentLogger())

	assert.Equal(t, "get_query_summary", tool.Tool.Name)
	assert.Contains(t, tool.Tool.InputSchema.Required, "query_run_id")
	assert.NotContains(t, tool.Tool.InputSchema.Properties, "after")
	assert.NotContains(t, tool.Tool.InputSchema.Properties, "limit")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "selectable")
	assert.Contains(t, tool.Tool.OutputSchema.Properties, "lists")
	outputSchema, err := json.Marshal(tool.Tool.OutputSchema)
	require.NoError(t, err)
	assert.NotContains(t, string(outputSchema), `"configuration"`)
	assert.Contains(t, string(outputSchema), `"configuration_attributes"`)
	assert.Contains(t, string(outputSchema), `"configuration_generated"`)
	assert.NotContains(t, tool.Tool.OutputSchema.Properties, "log_digest")
	assert.NotContains(t, tool.Tool.OutputSchema.Properties, "returned")
	assert.Contains(t, tool.Tool.Description, "get_query_status")
	assert.Contains(t, tool.Tool.Description, "query_result_marker")
	assert.Contains(t, tool.Tool.Description, "configuration_attributes")
	assert.Contains(t, tool.Tool.Description, "raw resource objects")
	assert.Contains(t, tool.Tool.Description, "returned by prepare_import")
	assert.Contains(t, tool.Tool.Description, "untrusted, review-required")
	assert.Contains(t, tool.Tool.Description, "generated import blocks")
	assert.Contains(t, tool.Tool.Description, "presentation only")
	assert.Contains(t, tool.Tool.Description, "report selectable results and stop")
	assert.Contains(t, tool.Tool.Description, "target workspace or local path")
	assert.Contains(t, tool.Tool.Description, "never select a first-N subset")
	assert.Contains(t, tool.Tool.Description, "nonselectable result contains no markers")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
}
