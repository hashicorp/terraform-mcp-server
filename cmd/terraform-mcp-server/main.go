// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	_ "embed"
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tfmcpserver "github.com/hashicorp/terraform-mcp-server/pkg/server"
	"github.com/hashicorp/terraform-mcp-server/version"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func runHTTPServer(logger *log.Logger, host string, port string, endpointPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hcServer := tfmcpserver.NewServer(version.Version, logger)
	return streamableHTTPServerInit(ctx, hcServer, logger, host, port, endpointPath)
}

func runStdioServer(logger *log.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hcServer := tfmcpserver.NewServer(version.Version, logger)
	return serverInit(ctx, hcServer, logger)
}

// runDefaultCommand handles the default behavior when no subcommand is provided
func runDefaultCommand(cmd *cobra.Command, _ []string) {
	// Default to stdio mode when no subcommand is provided
	logFile, err := cmd.PersistentFlags().GetString("log-file")
	if err != nil {
		stdlog.Fatal("Failed to get log file:", err)
	}
	logger, err := initLogger(logFile)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}

	if err := runStdioServer(logger); err != nil {
		stdlog.Fatal("failed to run stdio server:", err)
	}
}

func main() {
	// Check environment variables first - they override command line args
	if shouldUseStreamableHTTPMode() {
		port := getHTTPPort()
		host := getHTTPHost()
		endpointPath := getEndpointPath(nil)

		logFile, _ := rootCmd.PersistentFlags().GetString("log-file")
		logger, err := initLogger(logFile)
		if err != nil {
			stdlog.Fatal("Failed to initialize logger:", err)
		}

		if err := runHTTPServer(logger, host, port, endpointPath); err != nil {
			stdlog.Fatal("failed to run StreamableHTTP server:", err)
		}
		return
	}

	// Fall back to normal CLI behavior
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// shouldUseStreamableHTTPMode checks if environment variables indicate HTTP mode
func shouldUseStreamableHTTPMode() bool {
	transportMode := os.Getenv("TRANSPORT_MODE")
	return transportMode == "http" || transportMode == "streamable-http" ||
		os.Getenv("TRANSPORT_PORT") != "" ||
		os.Getenv("TRANSPORT_HOST") != "" ||
		os.Getenv("MCP_ENDPOINT") != ""
}

// shouldUseStatelessMode returns true if the MCP_SESSION_MODE environment variable is set to "stateless"
func shouldUseStatelessMode() bool {
	mode := strings.ToLower(os.Getenv("MCP_SESSION_MODE"))
	return mode == "stateless"
}

// getHTTPPort returns the port from environment variables or default
func getHTTPPort() string {
	if port := os.Getenv("TRANSPORT_PORT"); port != "" {
		return port
	}
	return "8080"
}

// getHTTPHost returns the host from environment variables or default
func getHTTPHost() string {
	if host := os.Getenv("TRANSPORT_HOST"); host != "" {
		return host
	}
	return "127.0.0.1"
}

// Add function to get endpoint path from environment or flag
func getEndpointPath(cmd *cobra.Command) string {
	// First check environment variable
	if envPath := os.Getenv("MCP_ENDPOINT"); envPath != "" {
		return envPath
	}

	// Fall back to command line flag
	if cmd != nil {
		if path, err := cmd.Flags().GetString("mcp-endpoint"); err == nil && path != "" {
			return path
		}
	}

	return "/mcp"
}
