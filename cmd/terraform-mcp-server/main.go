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

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//go:embed instructions.md
var instructions string

func runHTTPServer(logger *log.Logger, host string, port string, endpointPath string, enabledToolsets []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hcServer := NewServer(version.Version, logger, enabledToolsets)
	registerToolsAndResources(hcServer, logger, enabledToolsets)

	return streamableHTTPServerInit(ctx, hcServer, logger, host, port, endpointPath)
}

func runStdioServer(logger *log.Logger, enabledToolsets []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hcServer := NewServer(version.Version, logger, enabledToolsets)
	registerToolsAndResources(hcServer, logger, enabledToolsets)

	return serverInit(ctx, hcServer, logger)
}

func NewServer(version string, logger *log.Logger, enabledToolsets []string, opts ...server.ServerOption) *server.MCPServer {
	// Create rate limiting middleware with environment-based configuration
	rateLimitConfig := client.LoadRateLimitConfigFromEnv()
	rateLimitMiddleware := client.NewRateLimitMiddleware(rateLimitConfig, logger)

	// Add default options
	defaultOpts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithInstructions(instructions),
		server.WithToolHandlerMiddleware(rateLimitMiddleware.Middleware()),
		server.WithElicitation(),
	}
	opts = append(defaultOpts, opts...)

	// Create hooks for session management
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		client.NewSessionHandler(ctx, session, logger)
	})
	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		client.EndSessionHandler(ctx, session, logger)
	})

	// Add hooks to options
	opts = append(opts, server.WithHooks(hooks))

	// Create a new MCP server
	s := server.NewMCPServer(
		"terraform-mcp-server",
		version,
		opts...,
	)
	return s
}

// parseToolsets parses and validates the toolsets flag value
func parseToolsets(toolsetsFlag string, logger *log.Logger) []string {
	rawToolsets := strings.Split(toolsetsFlag, ",")

	cleaned, invalid := toolsets.CleanToolsets(rawToolsets)
	if len(invalid) > 0 {
		logger.Warnf("Invalid toolsets ignored: %v", invalid)
	}

	expanded := toolsets.ExpandDefaultToolset(cleaned)

	logger.Infof("Enabled toolsets: %v", expanded)
	return expanded
}

func getToolsetsFromCmd(cmd *cobra.Command, logger *log.Logger) []string {
	toolsetsFlag, err := cmd.Flags().GetString("toolsets")
	if err != nil {
		toolsetsFlag, err = cmd.Root().PersistentFlags().GetString("toolsets")
		if err != nil {
			logger.Warnf("Failed to get toolsets flag, using default: %v", err)
			toolsetsFlag = "default"
		}
	}
	return parseToolsets(toolsetsFlag, logger)
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

	// Get toolsets from the command that was passed in
	enabledToolsets := getToolsetsFromCmd(cmd, logger)

	if err := runStdioServer(logger, enabledToolsets); err != nil {
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

		enabledToolsets := getToolsetsFromCmd(rootCmd, logger)

		if err := runHTTPServer(logger, host, port, endpointPath, enabledToolsets); err != nil {
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

	// Explicitly check for "stateless" value
	if mode == "stateless" {
		return true
	}

	// All other values (including empty string, "stateful", or any other value) default to stateful mode
	return false
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
