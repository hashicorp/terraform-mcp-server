// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package toolsets

import "strings"

const (
	// Core toolsets
	Registry        = "registry"
	RegistryPrivate = "registry-private" // Private registry (TFE/TFC)
	Terraform       = "terraform"        // TFE/TFC operations

	// Special toolsets
	All     = "all"
	Default = "default"
)

// Toolset represents metadata about a toolset
type Toolset struct {
	Name        string
	Description string
}

var (
	AllToolset = Toolset{
		Name:        All,
		Description: "Special toolset that enables all available toolsets",
	}
	DefaultToolset = Toolset{
		Name:        Default,
		Description: "Special toolset that enables the default toolset configuration",
	}
	RegistryToolset = Toolset{
		Name:        Registry,
		Description: "Public Terraform Registry (providers, modules, policies)",
	}
	RegistryPrivateToolset = Toolset{
		Name:        RegistryPrivate,
		Description: "Private registry access (TFE/TFC private modules and providers)",
	}
	TerraformToolset = Toolset{
		Name:        Terraform,
		Description: "HCP Terraform/TFE operations (workspaces, runs, variables, etc.)",
	}
)

func AvailableToolsets() []Toolset {
	return []Toolset{
		RegistryToolset,
		RegistryPrivateToolset,
		TerraformToolset,
	}
}

// DefaultToolsets returns the default set of enabled toolsets
func DefaultToolsets() []string {
	return []string{Registry}
}

func GetValidToolsetNames() map[string]bool {
	validNames := make(map[string]bool)
	for _, ts := range AvailableToolsets() {
		validNames[ts.Name] = true
	}
	validNames[AllToolset.Name] = true
	validNames[DefaultToolset.Name] = true
	return validNames
}

func CleanToolsets(enabledToolsets []string) ([]string, []string) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(enabledToolsets))
	invalid := make([]string, 0)
	validNames := GetValidToolsetNames()

	for _, toolset := range enabledToolsets {
		trimmed := strings.TrimSpace(toolset)
		if trimmed == "" {
			continue
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
			if !validNames[trimmed] {
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
		if ts == Default {
			hasDefault = true
		}
	}

	if !hasDefault {
		return toolsets
	}

	result := make([]string, 0, len(toolsets))
	for _, ts := range toolsets {
		if ts != Default {
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
		toolsetNames = append(toolsetNames, ts.Name)
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
