// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

var searchTools = []ToolDef{
	{Name: "provider_list_schema_list", Toolset: Search, RequiresTFE: true},
}

// IsSearchTool reports whether name belongs to the Search toolset.
func IsSearchTool(name string) bool {
	for _, tool := range searchTools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
