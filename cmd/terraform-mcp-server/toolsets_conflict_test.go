// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"io"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fatalRecordingLogger returns a logrus logger whose Fatal calls flip the
// returned bool instead of terminating the process, letting tests assert on
// Fatal behavior without exiting the test binary.
func fatalRecordingLogger() (*log.Logger, *bool) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	fatalCalled := false
	logger.ExitFunc = func(_ int) { fatalCalled = true }
	return logger, &fatalCalled
}

// restoreToolsAndToolsetsFlags returns rootCmd's --tools / --toolsets persistent
// flags to their declared defaults and clears Changed so they do not leak into
// other tests in the package.
func restoreToolsAndToolsetsFlags(t *testing.T) {
	t.Helper()
	for _, spec := range []struct{ name, def string }{
		{"tools", ""},
		{"toolsets", "all"},
	} {
		f := rootCmd.PersistentFlags().Lookup(spec.name)
		if f == nil {
			continue
		}
		if err := rootCmd.PersistentFlags().Set(spec.name, spec.def); err != nil {
			t.Fatalf("failed to reset persistent flag %q: %v", spec.name, err)
		}
		f.Changed = false
	}
}

// TestToolsFlagAloneDoesNotConflictWithDefaultToolsets verifies that passing
// only --tools (without --toolsets) does not trigger the conflict Fatal in
// getToolsetsFromCmd.
//
// The conflict check at cmd/terraform-mcp-server/main.go reads the --toolsets
// value with GetString and treats anything non-empty / non-"default" as
// "explicitly set". Because --toolsets is declared with default "all", that
// check fires even when the operator did not pass --toolsets at all, and
// `./terraform-mcp-server --tools=X` exits with
//
//	Cannot use both --tools and --toolsets flags together
//
// The correct check is "did the user pass --toolsets?", which pflag exposes
// directly via flag.Changed.
func TestToolsFlagAloneDoesNotConflictWithDefaultToolsets(t *testing.T) {
	origRun := rootCmd.Run
	origArgs := os.Args
	t.Cleanup(func() {
		rootCmd.Run = origRun
		os.Args = origArgs
		restoreToolsAndToolsetsFlags(t)
	})

	os.Args = []string{
		"terraform-mcp-server",
		"--tools=search_providers,get_provider_details",
	}

	logger, fatalCalled := fatalRecordingLogger()
	var captured []string
	rootCmd.Run = func(cmd *cobra.Command, _ []string) {
		captured = getToolsetsFromCmd(cmd, logger)
	}

	require.NoError(t, rootCmd.Execute())

	assert.False(t, *fatalCalled,
		"passing only --tools (with --toolsets at its declared 'all' default) must not "+
			"trigger the --tools/--toolsets conflict Fatal")
	assert.Contains(t, captured, "search_providers")
	assert.Contains(t, captured, "get_provider_details")
}

// TestToolsAndToolsetsTogetherTriggersFatal locks the negative behavior down:
// when the operator explicitly passes both --tools and --toolsets, the
// conflict Fatal must still fire. This is the behavior the original check
// intended; the fix narrows the trigger to "user explicitly set --toolsets"
// without removing it.
func TestToolsAndToolsetsTogetherTriggersFatal(t *testing.T) {
	origRun := rootCmd.Run
	origArgs := os.Args
	t.Cleanup(func() {
		rootCmd.Run = origRun
		os.Args = origArgs
		restoreToolsAndToolsetsFlags(t)
	})

	os.Args = []string{
		"terraform-mcp-server",
		"--tools=search_providers",
		"--toolsets=registry",
	}

	logger, fatalCalled := fatalRecordingLogger()
	rootCmd.Run = func(cmd *cobra.Command, _ []string) {
		_ = getToolsetsFromCmd(cmd, logger)
	}

	require.NoError(t, rootCmd.Execute())

	assert.True(t, *fatalCalled,
		"passing both --tools and --toolsets explicitly must still trigger the conflict Fatal")
}
