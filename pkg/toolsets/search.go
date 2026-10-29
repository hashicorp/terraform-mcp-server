// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

// IsSearchTool reports whether name belongs to the Search toolset.
func IsSearchTool(name string) bool {
	toolset, exists := GetToolsetForTool(name)
	return exists && toolset == Search
}
