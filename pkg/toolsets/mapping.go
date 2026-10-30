// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

import (
	"strings"
	"sync"
)

// toolsetIndex is a memoized "tool name -> toolset name" lookup, built once
// from AllTools (registry.go) instead of hand-maintaining a second map
var toolsetIndex = sync.OnceValue(func() map[string]string {
	index := make(map[string]string, len(AllTools))
	for _, td := range AllTools {
		index[td.Name] = td.Toolset
	}
	return index
})

// GetToolsetForTool returns the toolset name for a given tool name
func GetToolsetForTool(toolName string) (string, bool) {
	toolset, exists := toolsetIndex()[toolName]
	return toolset, exists
}

// KnownToolNames returns every tool name registered in toolsetIndex
func KnownToolNames() map[string]bool {
	index := toolsetIndex()
	validTools := make(map[string]bool, len(index))
	for toolName := range index {
		validTools[toolName] = true
	}
	return validTools
}

// ParseIndividualTools parses and validates individual tool names
// Returns the validated tool names and any invalid ones
func ParseIndividualTools(toolNames []string) ([]string, []string) {
	validToolNames := KnownToolNames()
	seen := make(map[string]bool)
	valid := make([]string, 0, len(toolNames))
	invalid := make([]string, 0)

	for _, name := range toolNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			if validToolNames[trimmed] {
				valid = append(valid, trimmed)
			} else {
				invalid = append(invalid, trimmed)
			}
		}
	}

	return valid, invalid
}
