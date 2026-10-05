// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

import "testing"

func TestToolFilter_EnabledTools(t *testing.T) {
	f := NewToolsetFilter([]string{Registry})

	all := f.EnabledTools(nil)
	if len(all) == 0 {
		t.Fatal("expected at least one enabled tool for the registry toolset")
	}
	for _, td := range all {
		if td.Toolset != Registry {
			t.Errorf("EnabledTools(nil) returned %q from toolset %q, want only %q", td.Name, td.Toolset, Registry)
		}
	}

	withCheck := f.EnabledTools(func(td ToolDef) bool { return td.RequiresTFE })
	if len(withCheck) != 0 {
		t.Errorf("expected no RequiresTFE tools in the registry toolset, got %d", len(withCheck))
	}
}

func TestToolFilter_EnabledTools_IndividualMode(t *testing.T) {
	f := NewIndividualToolFilter([]string{"whoami"})

	got := f.EnabledTools(nil)
	if len(got) != 1 || got[0].Name != "whoami" {
		t.Errorf("EnabledTools(nil) = %v, want exactly [whoami]", got)
	}
}
