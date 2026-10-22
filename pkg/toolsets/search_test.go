// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package toolsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchMetadataAndCredentialGating(t *testing.T) {
	filter := NewToolsetFilter([]string{Search})
	want := []ToolDef{
		{Name: "generate_query_configuration", Toolset: Search},
		{Name: "provider_list_schema_list", Toolset: Search, RequiresTFE: true},
		{Name: "execute_query", Toolset: Search, RequiresTFE: true},
		{Name: "get_query_status", Toolset: Search, RequiresTFE: true},
		{Name: "get_query_summary", Toolset: Search, RequiresTFE: true},
		{Name: "prepare_import", Toolset: Search, RequiresTFE: true},
		{Name: "verify_import_plan", Toolset: Search, RequiresTFE: true},
	}
	require.Equal(t, want, filter.EnabledTools(nil))
	require.Equal(t, want[:1], filter.EnabledTools(func(td ToolDef) bool { return !td.RequiresTFE }))
	require.Equal(t, want[1:], filter.EnabledTools(func(td ToolDef) bool { return td.RequiresTFE }))
	for _, td := range want {
		require.True(t, IsSearchTool(td.Name))
		group, ok := GetToolsetForTool(td.Name)
		require.True(t, ok)
		require.Equal(t, Search, group)
		require.True(t, NewIndividualToolFilter([]string{td.Name}).IsToolEnabled(td.Name))
		require.True(t, NewToolsetFilter([]string{All}).IsToolEnabled(td.Name))
		require.False(t, NewToolsetFilter([]string{Registry}).IsToolEnabled(td.Name))
		require.True(t, KnownToolNames()[td.Name])
	}
	require.False(t, IsSearchTool("search_provider_docs"))
	require.False(t, IsSearchTool("unknown_tool"))
}
