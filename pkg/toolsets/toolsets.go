// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

import "strings"

// Toolset represents a group of related tools
type Toolset string

const (
	// Core toolsets
	ToolsetRegistry        Toolset = "registry"         // Public Terraform Registry
	ToolsetRegistryPrivate Toolset = "registry-private" // Private registry (TFE/TFC)
	ToolsetTerraform       Toolset = "terraform"        // TFE/TFC operations

	// Special toolsets
	ToolsetAll     Toolset = "all"
	ToolsetDefault Toolset = "default"
)

type ToolsetMetadata struct {
	ID          string
	Description string
}

var (
	MetadataAll = ToolsetMetadata{
		ID:          string(ToolsetAll),
		Description: "Special toolset that enables all available toolsets",
	}
	MetadataDefault = ToolsetMetadata{
		ID:          string(ToolsetDefault),
		Description: "Special toolset that enables the default toolset configuration",
	}
	MetadataRegistry = ToolsetMetadata{
		ID:          string(ToolsetRegistry),
		Description: "Public Terraform Registry (providers, modules, policies)",
	}
	MetadataRegistryPrivate = ToolsetMetadata{
		ID:          string(ToolsetRegistryPrivate),
		Description: "Private registry access (TFE/TFC private modules and providers)",
	}
	MetadataTerraform = ToolsetMetadata{
		ID:          string(ToolsetTerraform),
		Description: "HCP Terraform/TFE operations (workspaces, runs, variables, etc.)",
	}
)

// AvailableToolsets returns all available toolsets
func AvailableToolsets() []ToolsetMetadata {
	return []ToolsetMetadata{
		MetadataRegistry,
		MetadataRegistryPrivate,
		MetadataTerraform,
	}
}

// DefaultToolsets returns the default set of enabled toolsets
func DefaultToolsets() []string {
	return []string{
		string(ToolsetRegistry),
	}
}

// GetValidToolsetIDs returns a map of all valid toolset IDs
func GetValidToolsetIDs() map[string]bool {
	validIDs := make(map[string]bool)
	for _, ts := range AvailableToolsets() {
		validIDs[ts.ID] = true
	}
	validIDs[MetadataAll.ID] = true
	validIDs[MetadataDefault.ID] = true
	return validIDs
}

// CleanToolsets sanitizes the user input
func CleanToolsets(enabledToolsets []string) ([]string, []string) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(enabledToolsets))
	invalid := make([]string, 0)
	validIDs := GetValidToolsetIDs()

	for _, toolset := range enabledToolsets {
		trimmed := strings.TrimSpace(toolset)
		if trimmed == "" {
			continue
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
			if !validIDs[trimmed] {
				invalid = append(invalid, trimmed)
			}
		}
	}

	return result, invalid
}

func ExpandDefaultToolset(toolsets []string) []string {
	hasDefault := false
	seen := make(map[string]bool)

	for _, ts := range toolsets {
		seen[ts] = true
		if ts == string(ToolsetDefault) {
			hasDefault = true
		}
	}

	if !hasDefault {
		return toolsets
	}

	result := make([]string, 0, len(toolsets))
	for _, ts := range toolsets {
		if ts != string(ToolsetDefault) {
			result = append(result, ts)
		}
	}

	for _, defaultTS := range DefaultToolsets() {
		if !seen[defaultTS] {
			result = append(result, defaultTS)
		}
	}

	return result
}

// ContainsToolset checks if a toolset is in the list
func ContainsToolset(toolsets []string, toCheck string) bool {
	for _, ts := range toolsets {
		if ts == toCheck {
			return true
		}
	}
	return false
}

// GenerateToolsetsHelp generates help text for the toolsets flag
func GenerateToolsetsHelp() string {
	defaultTools := strings.Join(DefaultToolsets(), ", ")

	allToolsets := AvailableToolsets()
	var toolsetNames []string
	for _, ts := range allToolsets {
		toolsetNames = append(toolsetNames, ts.ID)
	}
	availableTools := strings.Join(toolsetNames, ", ")

	return "Comma-separated list of tool groups to enable.\n" +
		"Available: " + availableTools + "\n" +
		"Special toolset keywords:\n" +
		"  - all: Enables all available toolsets\n" +
		"  - default: Enables the default toolset configuration (" + defaultTools + ")\n" +
		"Examples:\n" +
		"  - --toolsets=registry,terraform\n" +
		"  - --toolsets=default,registry-private\n" +
		"  - --toolsets=all"
}
