// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTerraformOperationsEnabled(t *testing.T) {
	// Save original env var
	originalValue := os.Getenv("ENABLE_TF_OPERATIONS")
	defer os.Setenv("ENABLE_TF_OPERATIONS", originalValue)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"unset", "", false},
		{"false", "false", false},
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"True", "True", true},
		{"invalid", "invalid", false},
		{"1", "1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("ENABLE_TF_OPERATIONS")
			} else {
				os.Setenv("ENABLE_TF_OPERATIONS", tt.envValue)
			}
			assert.Equal(t, tt.expected, isTerraformOperationsEnabled())
		})
	}
}
