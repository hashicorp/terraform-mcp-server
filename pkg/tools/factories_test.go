// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
)

// toolsHandledOutsideFactoryMap are the two documented exceptions from
// factories.go that are registered by hand instead of via toolFactories.
var toolsHandledOutsideFactoryMap = map[string]bool{
	"create_no_code_workspace": true,
	"create_run":               true,
}

// TestToolFactoriesStayInSyncWithAllTools helps keep toolsets.AllTools and toolFactories in sync
// This test fails when someone adds a tool to registry.go and forgets to add its builder to factories.go or vice versa
func TestToolFactoriesStayInSyncWithAllTools(t *testing.T) {
	known := make(map[string]bool, len(toolsets.AllTools))
	for _, td := range toolsets.AllTools {
		known[td.Name] = true
		if toolsHandledOutsideFactoryMap[td.Name] {
			continue
		}
		if _, ok := toolFactories[td.Name]; !ok {
			t.Errorf("toolsets.AllTools has %q but toolFactories has no entry for it", td.Name)
		}
	}

	for name := range toolFactories {
		if !known[name] {
			t.Errorf("toolFactories has %q but toolsets.AllTools does not know about it", name)
		}
	}
}
