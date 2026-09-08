package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// TestMain builds the image once, runs the package tests, and cleans up leftovers.
func TestMain(m *testing.M) {
	// Build once so individual tests only pay for container startup.
	fmt.Print("Building Docker image for e2e tests...")
	buildCmd := exec.Command("make", "VERSION=test-e2e", "docker-build")
	buildCmd.Dir = ".." // Run this in the context of the root, where the Makefile is located.

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build E2E Docker image: %v\n%s\n", err, output)
		os.Exit(1)
	}

	// m.Run executes every Test function in this package.
	exitCode := m.Run()

	// Stop containers left behind by a failed test before exiting.
	cleanupTestContainers()
	os.Exit(exitCode)
}
