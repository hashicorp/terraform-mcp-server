// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	mcpmark3labs "github.com/hashicorp/terraform-mcp-server/pkg/mcp-mark3labs"
	"github.com/hashicorp/terraform-mcp-server/pkg/otelmetrics"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// parseToolsets parses and validates the toolsets flag value
func parseToolsets(toolsetsFlag string, logger *log.Logger) toolsets.ToolFilter {
	rawToolsets := strings.Split(toolsetsFlag, ",")

	cleaned, invalid := toolsets.CleanToolsets(rawToolsets)
	if len(invalid) > 0 {
		logger.Warnf("Invalid toolsets ignored: %v", invalid)
	}

	expanded := toolsets.ExpandDefaultToolset(cleaned)

	logger.Infof("Enabled toolsets: %v", expanded)
	return toolsets.NewToolsetFilter(expanded)
}

// parseIndividualTools parses and validates the tools flag value
func parseIndividualTools(toolsFlag string, logger *log.Logger) toolsets.ToolFilter {
	rawTools := strings.Split(toolsFlag, ",")

	validTools, invalidTools := toolsets.ParseIndividualTools(rawTools)
	if len(invalidTools) > 0 {
		logger.Warnf("Invalid tool names ignored: %v", invalidTools)
	}

	if len(validTools) == 0 {
		logger.Warn("No valid tools specified, falling back to default toolsets")
		return parseToolsets(toolsets.Default, logger)
	}
	logger.Infof("Enabled individual tools: %v", validTools)
	return toolsets.NewIndividualToolFilter(validTools)
}

func getToolsetsFromCmd(cmd *cobra.Command, logger *log.Logger) toolsets.ToolFilter {
	// Check if --tools flag is set (individual tool mode)
	toolsFlag, err := cmd.Flags().GetString("tools")
	if err != nil {
		// Try root persistent flags
		toolsFlag, err = cmd.Root().PersistentFlags().GetString("tools")
	}

	if err == nil && toolsFlag != "" {
		// Ensure --toolsets is not also explicitly set. The flag is declared with
		// a non-empty default ("all"), so a value-based check would always fire
		// when only --tools is passed; pflag's Changed field is the precise
		// "was this flag set on the command line?" signal.
		toolsetsFlagDef := cmd.Flags().Lookup("toolsets")
		if toolsetsFlagDef == nil {
			toolsetsFlagDef = cmd.Root().PersistentFlags().Lookup("toolsets")
		}
		if toolsetsFlagDef != nil && toolsetsFlagDef.Changed {
			logger.Fatal("Cannot use both --tools and --toolsets flags together")
		}
		return parseIndividualTools(toolsFlag, logger)
	}

	// Fall back to toolsets mode
	toolsetsFlag, err := cmd.Flags().GetString("toolsets")
	if err != nil {
		toolsetsFlag, err = cmd.Root().PersistentFlags().GetString("toolsets")
		if err != nil {
			logger.Warnf("Failed to get toolsets flag, using default: %v", err)
			toolsetsFlag = toolsets.Default
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
	logLevel := logging.LevelFromCommand(cmd)
	logFormat := logging.FormatFromCommand(cmd)
	logger, err := logging.NewLogger(logFile, logLevel, logFormat)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}

	// Get toolsets from the command that was passed in
	filter := getToolsetsFromCmd(cmd, logger)

	if err := mcpmark3labs.RunStdioServer(logger, filter); err != nil {
		stdlog.Fatal("failed to run stdio server:", err)
	}
}

// Run is the terraform-mcp-server entrypoint: it resolves logging, decides
// between the env-var-driven StreamableHTTP fast path and normal cobra CLI
// behavior, and starts the appropriate server. It only returns after the
// process would otherwise exit cleanly; error paths exit the process
// directly (matching the original main()'s behavior).
func Run() {
	logFile, _ := rootCmd.PersistentFlags().GetString("log-file")
	logLevel := logging.LevelFromCommand(rootCmd)
	logFormat := logging.FormatFromCommand(rootCmd)
	logger, err := logging.NewLogger(logFile, logLevel, logFormat)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}
	if shouldUseStreamableHTTPMode() {
		logger.Info("Starting in Streamable HTTP mode based on environment configuration")

		metricsConfig, shutdownMetrics := otelmetrics.Setup(logger)
		defer shutdownMetrics()

		port := getHTTPPort()
		host := getHTTPHost()
		endpointPath := getEndpointPath(nil)
		filter := getToolsetsFromCmd(rootCmd, logger)
		heartbeatInterval := getHeartbeatInterval()
		organizationAllowlist, err := getOrganizationAllowlist(rootCmd)
		if err != nil {
			stdlog.Fatal(err)
		}
		if err := mcpmark3labs.RunHTTPServer(logger, host, port, endpointPath, heartbeatInterval, filter, metricsConfig, organizationAllowlist, rootCmd); err != nil {
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

// getHeartbeatInterval returns the heartbeat interval duration from the env var or default
func getHeartbeatInterval() time.Duration {
	if val := os.Getenv("MCP_HEARTBEAT_INTERVAL"); val != "" {
		duration, err := time.ParseDuration(val)
		if err == nil {
			return duration
		}
	}
	return 0
}

func getOrganizationAllowlist(cmd *cobra.Command) ([]string, error) {
	if envAllowlist, ok := os.LookupEnv(client.OrganizationAllowlistEnv); ok {
		return client.ParseOrganizationAllowlistCSV(envAllowlist)
	}

	if cmd != nil {
		if cmd.Flags().Changed("organization-allowlist") {
			allowlist, err := cmd.Flags().GetString("organization-allowlist")
			if err != nil {
				return nil, err
			}
			return client.ParseOrganizationAllowlistCSV(allowlist)
		}
	}

	return nil, nil
}
