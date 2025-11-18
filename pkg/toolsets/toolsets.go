// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

import "strings"

// Toolset will represent the group of related tools
type Toolset string

const (
	// The Core toolsets
	ToolsetProviders       Toolset = "providers"
	ToolsetModules         Toolset = "modules"
	ToolsetPolicies        Toolset = "policies"
	ToolsetWorkspaces      Toolset = "workspaces"
	ToolsetRuns            Toolset = "runs"
	ToolsetVariables       Toolset = "variables"
	ToolsetVariableSets    Toolset = "variable_sets"
	ToolsetTags            Toolset = "tags"
	ToolsetOrganizations   Toolset = "organizations"
	ToolsetProjects        Toolset = "projects"
	ToolsetPrivateRegistry Toolset = "private_registry"

	// The Special toolsets
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
	MetadataProviders = ToolsetMetadata{
		ID:          string(ToolsetProviders),
		Description: "Terraform Registry provider documentation tools",
	}
	MetadataModules = ToolsetMetadata{
		ID:          string(ToolsetModules),
		Description: "Terraform Registry module discovery and documentation",
	}
	MetadataPolicies = ToolsetMetadata{
		ID:          string(ToolsetPolicies),
		Description: "Sentinel policy tools",
	}
	MetadataWorkspaces = ToolsetMetadata{
		ID:          string(ToolsetWorkspaces),
		Description: "HCP Terraform/TFE workspace management",
	}
	MetadataRuns = ToolsetMetadata{
		ID:          string(ToolsetRuns),
		Description: "Terraform run operations",
	}
	MetadataVariables = ToolsetMetadata{
		ID:          string(ToolsetVariables),
		Description: "Workspace variable management",
	}
	MetadataVariableSets = ToolsetMetadata{
		ID:          string(ToolsetVariableSets),
		Description: "Variable set management",
	}
	MetadataTags = ToolsetMetadata{
		ID:          string(ToolsetTags),
		Description: "Workspace tagging",
	}
	MetadataOrganizations = ToolsetMetadata{
		ID:          string(ToolsetOrganizations),
		Description: "Organization listing and management",
	}
	MetadataProjects = ToolsetMetadata{
		ID:          string(ToolsetProjects),
		Description: "Project listing and management",
	}
	MetadataPrivateRegistry = ToolsetMetadata{
		ID:          string(ToolsetPrivateRegistry),
		Description: "Private registry (modules and providers)",
	}
)

// returns all available toolsets
func AvailableToolsets() []ToolsetMetadata {
	return []ToolsetMetadata{
		MetadataProviders,
		MetadataModules,
		MetadataPolicies,
		MetadataWorkspaces,
		MetadataRuns,
		MetadataVariables,
		MetadataVariableSets,
		MetadataTags,
		MetadataOrganizations,
		MetadataProjects,
		MetadataPrivateRegistry,
	}
}

// returns the default set of enabled toolsets
func DefaultToolsets() []string {
	return []string{
		string(ToolsetProviders),
		string(ToolsetModules),
		string(ToolsetPolicies),
		string(ToolsetWorkspaces),
	}
}

// will help Determine if user input is valid
func GetValidToolsetIDs() map[string]bool {
	validIDs := make(map[string]bool)
	for _, ts := range AvailableToolsets() {
		validIDs[ts.ID] = true
	}
	validIDs[MetadataAll.ID] = true
	validIDs[MetadataDefault.ID] = true
	return validIDs
}

// Sanitizes the user input
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

	// Add default toolsets if its not already present
	for _, defaultTS := range DefaultToolsets() {
		if !seen[defaultTS] {
			result = append(result, defaultTS)
		}
	}

	return result
}

// Checks if a Toolset is in the list
func ContainsToolset(toolsets []string, toCheck string) bool {
	for _, ts := range toolsets {
		if ts == toCheck {
			return true
		}
	}
	return false
}

// Will generate the help text for the toolset flag
func GenerateToolsetsHelp() string {
	defaultTools := strings.Join(DefaultToolsets(), ", ")

	allToolsets := AvailableToolsets()
	var availableToolsLines []string
	const maxLineLength = 70
	currentLine := ""

	for i, ts := range allToolsets {
		switch {
		case i == 0:
			currentLine = ts.ID
		case len(currentLine)+len(ts.ID)+2 <= maxLineLength:
			currentLine += ", " + ts.ID
		default:
			availableToolsLines = append(availableToolsLines, currentLine)
			currentLine = ts.ID
		}
	}
	if currentLine != "" {
		availableToolsLines = append(availableToolsLines, currentLine)
	}

	availableTools := strings.Join(availableToolsLines, ",\n\t     ")

	return "Comma-separated list of tool groups to enable.\n" +
		"Available: " + availableTools + "\n" +
		"Special toolset keywords:\n" +
		"  - all: Enables all available toolsets\n" +
		"  - default: Enables the default toolset configuration of:\n\t     " + defaultTools + "\n" +
		"Examples:\n" +
		"  - --toolsets=providers,workspaces,runs\n" +
		"  - Default + additional: --toolsets=default,runs,variables\n" +
		"  - All tools: --toolsets=all"
}
