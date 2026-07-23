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

// resetPersistentFlag restores a rootCmd persistent flag to its declared default
// and clears its Changed status so it does not leak into other tests in the
// package. The flag is expected to exist; if not, the helper is a no-op.
func resetPersistentFlag(t *testing.T, name, defaultValue string) {
	t.Helper()
	flag := rootCmd.PersistentFlags().Lookup(name)
	if flag == nil {
		return
	}
	if err := rootCmd.PersistentFlags().Set(name, defaultValue); err != nil {
		t.Fatalf("failed to reset persistent flag %q: %v", name, err)
	}
	flag.Changed = false
}

// TestStreamableHTTPModeEnvVarRespectsToolsFlag is the regression test for #376.
//
// Before the fix, when the server was launched via the TRANSPORT_MODE /
// TRANSPORT_PORT / TRANSPORT_HOST / MCP_ENDPOINT env vars (rather than the
// explicit `streamable-http` subcommand), main() invoked runHTTPServer directly
// without first calling rootCmd.Execute(). Cobra therefore never parsed
// --tools or --toolsets and the full default catalog was silently enabled
// regardless of what the operator passed on the command line.
//
// The fix moved the env-var-driven HTTP path inside runDefaultCommand (the
// cobra Run of rootCmd), so Execute() always runs first. This test locks that
// contract in: when rootCmd.Execute() is invoked with --tools while the
// env-vars select streamable-HTTP mode, the cobra Run callback receives a
// command whose --tools value is readable via getToolsetsFromCmd, and the
// resolved toolset list is the operator's individual selection — not "all".
func TestStreamableHTTPModeEnvVarRespectsToolsFlag(t *testing.T) {
	t.Setenv("TRANSPORT_MODE", "streamable-http")

	origRun := rootCmd.Run
	origArgs := os.Args
	t.Cleanup(func() {
		rootCmd.Run = origRun
		os.Args = origArgs
		resetPersistentFlag(t, "tools", "")
		resetPersistentFlag(t, "toolsets", "all")
	})

	// --toolsets="" is needed to bypass an orthogonal conflict-check inside
	// getToolsetsFromCmd that triggers on the declared default "all"; it is
	// not part of the bug under test, just a side condition for invoking
	// --tools cleanly. The regression behavior we want to lock down is that
	// cobra parses --tools out of os.Args at all in this code path.
	os.Args = []string{
		"terraform-mcp-server",
		"--tools=search_providers,get_provider_details",
		"--toolsets=",
	}

	// Replace the default Run with a capture-only callback so the test does
	// not actually start a server. The callback mirrors the two reads
	// runDefaultCommand performs before deciding which transport to start.
	var capturedToolsets []string
	var sawHTTPMode bool
	rootCmd.Run = func(cmd *cobra.Command, _ []string) {
		logger := log.New()
		logger.SetOutput(io.Discard)
		capturedToolsets = getToolsetsFromCmd(cmd, logger)
		sawHTTPMode = shouldUseStreamableHTTPMode()
	}

	require.NoError(t, rootCmd.Execute())

	assert.True(t, sawHTTPMode,
		"TRANSPORT_MODE=streamable-http should select the HTTP branch inside runDefaultCommand")
	assert.Contains(t, capturedToolsets, "search_providers",
		"regression #376: --tools must be honored when env-var HTTP mode is selected")
	assert.Contains(t, capturedToolsets, "get_provider_details",
		"regression #376: --tools must be honored when env-var HTTP mode is selected")
	assert.NotContains(t, capturedToolsets, "all",
		"--tools should switch to individual-tool mode, not enable the full 'all' toolset")
}

// TestStreamableHTTPModeEnvVarRespectsToolsetsFlag is the --toolsets variant of
// the regression test for #376. The bug applied to both --tools and --toolsets;
// this test locks down that --toolsets is parsed in env-var HTTP mode too.
func TestStreamableHTTPModeEnvVarRespectsToolsetsFlag(t *testing.T) {
	t.Setenv("TRANSPORT_MODE", "streamable-http")

	origRun := rootCmd.Run
	origArgs := os.Args
	t.Cleanup(func() {
		rootCmd.Run = origRun
		os.Args = origArgs
		resetPersistentFlag(t, "tools", "")
		resetPersistentFlag(t, "toolsets", "all")
	})

	os.Args = []string{
		"terraform-mcp-server",
		"--toolsets=registry",
	}

	var capturedToolsets []string
	rootCmd.Run = func(cmd *cobra.Command, _ []string) {
		logger := log.New()
		logger.SetOutput(io.Discard)
		capturedToolsets = getToolsetsFromCmd(cmd, logger)
	}

	require.NoError(t, rootCmd.Execute())

	assert.Contains(t, capturedToolsets, "registry",
		"regression #376: --toolsets must be honored when env-var HTTP mode is selected")
	assert.NotContains(t, capturedToolsets, "all",
		"--toolsets=registry should not fall back to the 'all' default")
}
