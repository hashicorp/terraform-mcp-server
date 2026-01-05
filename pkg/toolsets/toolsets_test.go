// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

import (
	"reflect"
	"testing"
)

func TestCleanToolsets(t *testing.T) {
	tests := []struct {
		name            string
		input           []string
		expectedValid   []string
		expectedInvalid []string
	}{
		{
			name:            "valid toolsets",
			input:           []string{"registry", "terraform"},
			expectedValid:   []string{"registry", "terraform"},
			expectedInvalid: []string{},
		},
		{
			name:            "invalid toolsets",
			input:           []string{"invalid", "fake"},
			expectedValid:   []string{"invalid", "fake"},
			expectedInvalid: []string{"invalid", "fake"},
		},
		{
			name:            "mixed valid and invalid",
			input:           []string{"registry", "invalid", "terraform"},
			expectedValid:   []string{"registry", "invalid", "terraform"},
			expectedInvalid: []string{"invalid"},
		},
		{
			name:            "empty strings",
			input:           []string{"registry", "", "terraform", "  "},
			expectedValid:   []string{"registry", "terraform"},
			expectedInvalid: []string{},
		},
		{
			name:            "duplicates",
			input:           []string{"registry", "registry", "terraform"},
			expectedValid:   []string{"registry", "terraform"},
			expectedInvalid: []string{},
		},
		{
			name:            "whitespace trimming",
			input:           []string{" registry ", "  terraform  "},
			expectedValid:   []string{"registry", "terraform"},
			expectedInvalid: []string{},
		},
		{
			name:            "special toolsets",
			input:           []string{"all", "default"},
			expectedValid:   []string{"all", "default"},
			expectedInvalid: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, invalid := CleanToolsets(tt.input)

			if !reflect.DeepEqual(valid, tt.expectedValid) {
				t.Errorf("CleanToolsets() valid = %v, want %v", valid, tt.expectedValid)
			}

			if !reflect.DeepEqual(invalid, tt.expectedInvalid) {
				t.Errorf("CleanToolsets() invalid = %v, want %v", invalid, tt.expectedInvalid)
			}
		})
	}
}

func TestExpandDefaultToolset(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no default keyword",
			input:    []string{"registry", "terraform"},
			expected: []string{"registry", "terraform"},
		},
		{
			name:     "default keyword only",
			input:    []string{"default"},
			expected: []string{"registry"},
		},
		{
			name:     "default with additional toolsets",
			input:    []string{"default", "terraform"},
			expected: []string{"terraform", "registry"},
		},
		{
			name:     "default with registry already included",
			input:    []string{"default", "registry", "terraform"},
			expected: []string{"registry", "terraform"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandDefaultToolset(tt.input)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ExpandDefaultToolset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestContainsToolset(t *testing.T) {
	tests := []struct {
		name     string
		toolsets []string
		toCheck  string
		expected bool
	}{
		{
			name:     "toolset present",
			toolsets: []string{"registry", "terraform"},
			toCheck:  "registry",
			expected: true,
		},
		{
			name:     "toolset not present",
			toolsets: []string{"registry", "terraform"},
			toCheck:  "registry-private",
			expected: false,
		},
		{
			name:     "empty list",
			toolsets: []string{},
			toCheck:  "registry",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsToolset(tt.toolsets, tt.toCheck)

			if result != tt.expected {
				t.Errorf("ContainsToolset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetValidToolsetNames(t *testing.T) {
	validNames := GetValidToolsetNames()

	// Check that all expected toolsets are present
	expected := []string{"registry", "registry-private", "terraform", "all", "default"}
	for _, name := range expected {
		if !validNames[name] {
			t.Errorf("GetValidToolsetNames() missing expected toolset: %s", name)
		}
	}

	if len(validNames) != len(expected) {
		t.Errorf("GetValidToolsetNames() returned %d toolsets, want %d", len(validNames), len(expected))
	}
}

func TestIsToolEnabled(t *testing.T) {
	tests := []struct {
		name            string
		toolName        string
		enabledToolsets []string
		expected        bool
	}{
		{
			name:            "tool enabled - registry",
			toolName:        "search_providers",
			enabledToolsets: []string{"registry"},
			expected:        true,
		},
		{
			name:            "tool disabled",
			toolName:        "search_providers",
			enabledToolsets: []string{"terraform"},
			expected:        false,
		},
		{
			name:            "all toolset enables everything",
			toolName:        "search_providers",
			enabledToolsets: []string{"all"},
			expected:        true,
		},
		{
			name:            "unknown tool",
			toolName:        "unknown_tool",
			enabledToolsets: []string{"registry"},
			expected:        false,
		},
		{
			name:            "terraform tool",
			toolName:        "list_workspaces",
			enabledToolsets: []string{"terraform"},
			expected:        true,
		},
		{
			name:            "private registry tool",
			toolName:        "search_private_modules",
			enabledToolsets: []string{"registry-private"},
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsToolEnabled(tt.toolName, tt.enabledToolsets)

			if result != tt.expected {
				t.Errorf("IsToolEnabled(%s, %v) = %v, want %v", tt.toolName, tt.enabledToolsets, result, tt.expected)
			}
		})
	}
}

func TestParseIndividualTools(t *testing.T) {
	tests := []struct {
		name            string
		input           []string
		expectedValid   []string
		expectedInvalid []string
	}{
		{
			name:            "valid tools",
			input:           []string{"search_providers", "get_provider_details"},
			expectedValid:   []string{"search_providers", "get_provider_details"},
			expectedInvalid: []string{},
		},
		{
			name:            "invalid tools",
			input:           []string{"invalid_tool", "fake_tool"},
			expectedValid:   []string{},
			expectedInvalid: []string{"invalid_tool", "fake_tool"},
		},
		{
			name:            "mixed valid and invalid",
			input:           []string{"search_providers", "invalid_tool", "list_workspaces"},
			expectedValid:   []string{"search_providers", "list_workspaces"},
			expectedInvalid: []string{"invalid_tool"},
		},
		{
			name:            "empty strings",
			input:           []string{"search_providers", "", "list_workspaces", "  "},
			expectedValid:   []string{"search_providers", "list_workspaces"},
			expectedInvalid: []string{},
		},
		{
			name:            "duplicates",
			input:           []string{"search_providers", "search_providers", "list_workspaces"},
			expectedValid:   []string{"search_providers", "list_workspaces"},
			expectedInvalid: []string{},
		},
		{
			name:            "whitespace trimming",
			input:           []string{" search_providers ", "  list_workspaces  "},
			expectedValid:   []string{"search_providers", "list_workspaces"},
			expectedInvalid: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, invalid := ParseIndividualTools(tt.input)

			if !reflect.DeepEqual(valid, tt.expectedValid) {
				t.Errorf("ParseIndividualTools() valid = %v, want %v", valid, tt.expectedValid)
			}

			if !reflect.DeepEqual(invalid, tt.expectedInvalid) {
				t.Errorf("ParseIndividualTools() invalid = %v, want %v", invalid, tt.expectedInvalid)
			}
		})
	}
}

func TestIsToolEnabledIndividualMode(t *testing.T) {
	tests := []struct {
		name            string
		toolName        string
		enabledToolsets []string
		expected        bool
	}{
		{
			name:            "tool enabled in individual mode",
			toolName:        "search_providers",
			enabledToolsets: EnableIndividualTools([]string{"search_providers", "list_workspaces"}),
			expected:        true,
		},
		{
			name:            "tool disabled in individual mode",
			toolName:        "get_provider_details",
			enabledToolsets: EnableIndividualTools([]string{"search_providers", "list_workspaces"}),
			expected:        false,
		},
		{
			name:            "all toolset overrides individual mode",
			toolName:        "get_provider_details",
			enabledToolsets: append([]string{"all"}, EnableIndividualTools([]string{"search_providers"})...),
			expected:        true,
		},
		{
			name:            "terraform tool in individual mode",
			toolName:        "list_workspaces",
			enabledToolsets: EnableIndividualTools([]string{"list_workspaces"}),
			expected:        true,
		},
		{
			name:            "private registry tool in individual mode",
			toolName:        "search_private_modules",
			enabledToolsets: EnableIndividualTools([]string{"search_private_modules"}),
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsToolEnabled(tt.toolName, tt.enabledToolsets)

			if result != tt.expected {
				t.Errorf("IsToolEnabled(%s, %v) = %v, want %v", tt.toolName, tt.enabledToolsets, result, tt.expected)
			}
		})
	}
}

func TestGetAllValidToolNames(t *testing.T) {
	validTools := GetAllValidToolNames()

	// Verify we have a reasonable number of tools (at least the ones we know about)
	expectedTools := []string{
		"search_providers",
		"get_provider_details",
		"search_modules",
		"get_module_details",
		"search_policies",
		"get_policy_details",
		"list_workspaces",
		"create_workspace",
		"search_private_modules",
		"search_private_providers",
	}

	for _, tool := range expectedTools {
		if !validTools[tool] {
			t.Errorf("GetAllValidToolNames() missing expected tool: %s", tool)
		}
	}

	// Verify count matches ToolToToolset map
	if len(validTools) != len(ToolToToolset) {
		t.Errorf("GetAllValidToolNames() returned %d tools, want %d", len(validTools), len(ToolToToolset))
	}
}
