// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
)

// TestToolFactoriesStayInSyncWithAllTools helps keep toolsets.AllTools and toolFactories in sync
// This test fails when someone adds a tool to registry.go and forgets to add its builder to factories.go or vice versa
func TestToolFactoriesStayInSyncWithAllTools(t *testing.T) {
	known := make(map[string]bool, len(toolsets.AllTools))
	for _, td := range toolsets.AllTools {
		known[td.Name] = true
		_, plain := toolFactories[td.Name]
		_, special := specialFactories[td.Name]
		if !plain && !special {
			t.Errorf("toolsets.AllTools has %q but no factory builds it", td.Name)

		}
	}
	for name := range toolFactories {
		if !known[name] {
			t.Errorf("toolFactories has %q but toolsets.AllTools does not know about it", name)
		}
	}
	for name := range specialFactories {
		if !known[name] {
			t.Errorf("specialFactories has %q but toolsets.AllTools does not know about it", name)
		}
	}

}
