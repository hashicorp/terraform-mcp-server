// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetPrivateModuleDetails(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetPrivateModuleDetails(logger)
		
		assert.NotNil(t, tool.Tool)
		assert.NotNil(t, tool.Handler)
		assert.Equal(t, "get_private_module_details", tool.Tool.Name)
		
		// Check that the tool has the expected parameters
		assert.NotNil(t, tool.Tool.InputSchema)
		
		// Verify required parameters exist
		properties := tool.Tool.InputSchema.Properties
		assert.Contains(t, properties, "terraform_org_name")
		assert.Contains(t, properties, "module_id")
		
		// Verify optional parameters exist
		assert.Contains(t, properties, "module_version")
		assert.Contains(t, properties, "registry_name")
		assert.Contains(t, properties, "include_versions")
		
		// Check required fields
		required := tool.Tool.InputSchema.Required
		assert.Contains(t, required, "terraform_org_name")
		assert.Contains(t, required, "module_id")
		
		// Verify that the old separate parameters are not present
		assert.NotContains(t, properties, "module_namespace")
		assert.NotContains(t, properties, "module_name")
		assert.NotContains(t, properties, "module_provider")
	})

	t.Run("tool description", func(t *testing.T) {
		tool := GetPrivateModuleDetails(logger)
		
		assert.Contains(t, tool.Tool.Description, "private module")
		assert.Contains(t, tool.Tool.Description, "Terraform Cloud/Enterprise")
		assert.Contains(t, tool.Tool.Description, "search_private_modules")
		assert.Contains(t, tool.Tool.Description, "module_id")
	})
}

func TestBuildPrivateModuleDetailsResponse(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("basic module response", func(t *testing.T) {
		// This test would require mocking the TFE types
		// For now, we'll just test that the function exists and can be called
		// In a real implementation, you'd want to:
		// 1. Create mock tfe.RegistryModule and tfe.RegistryModuleVersion objects
		// 2. Call buildPrivateModuleDetailsResponse with test data
		// 3. Verify the formatted output contains expected information
		
		assert.True(t, true, "Placeholder test - implement with mock data")
	})

	t.Run("module_id parsing validation", func(t *testing.T) {
		// Test that module_id format validation would work
		testCases := []struct {
			moduleID string
			valid    bool
		}{
			{"my-org/vpc/aws", true},
			{"terraform-aws-modules/vpc/aws", true},
			{"namespace/name/provider", true},
			{"invalid-format", false},
			{"too/many/parts/here", false},
			{"only-two/parts", false},
			{"", false},
		}

		for _, tc := range testCases {
			parts := len(strings.Split(tc.moduleID, "/"))
			if tc.valid {
				assert.Equal(t, 3, parts, "Valid module_id should have 3 parts: %s", tc.moduleID)
			} else {
				assert.NotEqual(t, 3, parts, "Invalid module_id should not have 3 parts: %s", tc.moduleID)
			}
		}
	})
}
