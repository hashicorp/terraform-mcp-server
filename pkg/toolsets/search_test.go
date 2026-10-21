// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package toolsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchProviderMetadataAndCredentialGating(t *testing.T) {
	filter := NewToolsetFilter([]string{Search})
	want := []ToolDef{{Name: "provider_list_schema_list", Toolset: Search, RequiresTFE: true}}
	require.Equal(t, want, filter.EnabledTools(nil))
	require.Empty(t, filter.EnabledTools(func(td ToolDef) bool { return !td.RequiresTFE }))
	require.Equal(t, want, filter.EnabledTools(func(td ToolDef) bool { return td.RequiresTFE }))
	require.True(t, IsSearchTool("provider_list_schema_list"))
	require.False(t, IsSearchTool("search_provider_docs"))
	require.False(t, IsSearchTool("unknown_tool"))
	group, ok := GetToolsetForTool("provider_list_schema_list")
	require.True(t, ok)
	require.Equal(t, Search, group)
}
