// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

func TestAnalyzeAndFormatCapabilities(t *testing.T) {
	// Mock provider docs
	mockDocs := client.ProviderDocs{
		Docs: []client.ProviderDoc{
			{Category: "resources", Title: "aws_instance", Language: "hcl"},
			{Category: "resources", Title: "aws_s3_bucket", Language: "hcl"},
			{Category: "data-sources", Title: "aws_ami", Language: "hcl"},
			{Category: "functions", Title: "base64encode", Language: "hcl"},
			{Category: "guides", Title: "Getting Started", Language: "hcl"},
		},
	}

	result := analyzeAndFormatCapabilities(mockDocs, "hashicorp", "aws", "5.0.0")

	// Basic checks
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Check that it contains expected sections
	expectedSections := []string{"Resources:", "Data Sources:", "Functions:", "Guides:"}
	for _, section := range expectedSections {
		if !contains(result, section) {
			t.Errorf("Expected result to contain '%s'", section)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		(len(s) > len(substr) && contains(s[1:], substr))
}
