// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

// FilterMode selects how ToolFilter.IsToolEnabled decides availability.
type FilterMode int

const (
	// ModeToolsets enables tools by their toolset membership (Toolsets field).
	ModeToolsets FilterMode = iota
	// ModeIndividualTools enables only the exact tool names in Tools.
	ModeIndividualTools
)

// ToolFilter replaces the old []string + sentinel-marker convention for
// passing enabled-tool state through the app. Construct with
// NewToolsetFilter or NewIndividualToolFilter
type ToolFilter struct {
	Mode     FilterMode
	Toolsets []string
	Tools    []string
}

func NewToolsetFilter(toolsets []string) ToolFilter {
	return ToolFilter{Mode: ModeToolsets, Toolsets: toolsets}
}

func NewIndividualToolFilter(tools []string) ToolFilter {
	return ToolFilter{Mode: ModeIndividualTools, Tools: tools}
}

// IsToolEnabled reports whether toolName is enabled under this filter
func (f ToolFilter) IsToolEnabled(toolName string) bool {
	if f.Mode == ModeIndividualTools {
		return ContainsToolset(f.Tools, toolName)
	}
	if ContainsToolset(f.Toolsets, All) {
		return true
	}
	toolset, exists := GetToolsetForTool(toolName)
	if !exists {
		return false
	}
	return ContainsToolset(f.Toolsets, toolset)
}
