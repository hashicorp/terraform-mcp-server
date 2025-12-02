// Copyright (c) HashiCorp, Inc.
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
