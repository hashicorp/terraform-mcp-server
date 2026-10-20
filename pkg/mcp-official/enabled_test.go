// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"os"
	"testing"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		name  string
		unset bool   // true means the variable is not set at all
		value string // used when unset is false
		want  bool
	}{
		{name: "unset uses the official server", unset: true, want: true},
		{name: "empty uses the official server", value: "", want: true},
		{name: "true", value: "true", want: true},
		{name: "1", value: "1", want: true},
		{name: "false falls back to legacy", value: "false", want: false},
		{name: "FALSE falls back to legacy", value: "FALSE", want: false},
		{name: "0 falls back to legacy", value: "0", want: false},
		{name: "spaces around the value are ignored", value: "  false  ", want: false},
		{name: "garbage keeps the default", value: "nope", want: true},
		{name: "a typo of false keeps the default", value: "flase", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv restores the original value when the test ends. Unsetting
			// right after it makes the variable truly absent for this case.
			t.Setenv(EnvEnabled, tt.value)
			if tt.unset {
				if err := os.Unsetenv(EnvEnabled); err != nil {
					t.Fatal(err)
				}
			}

			if got := Enabled(); got != tt.want {
				t.Fatalf("Enabled() with %s=%q (unset=%v) = %v, want %v", EnvEnabled, tt.value, tt.unset, got, tt.want)
			}
		})
	}
}
