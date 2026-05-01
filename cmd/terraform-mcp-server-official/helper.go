package main

import (
	"fmt"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

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

// parseIndividualTools parses and validates the tools flag value
func parseIndividualTools(toolsFlag string, logger *log.Logger) []string {
	rawTools := strings.Split(toolsFlag, ",")

	validTools, invalidTools := toolsets.ParseIndividualTools(rawTools)
	if len(invalidTools) > 0 {
		logger.Warnf("Invalid tool names ignored: %v", invalidTools)
	}

	if len(validTools) == 0 {
		logger.Warn("No valid tools specified, falling back to default toolsets")
		return parseToolsets("default", logger)
	}

	// Use the public API to enable individual tools mode
	result := toolsets.EnableIndividualTools(validTools)
	logger.Infof("Enabled individual tools: %v", validTools)
	return result
}

func getToolsetsFromCmd(cmd *cobra.Command, logger *log.Logger) []string {
	// Check if --tools flag is set (individual tool mode)
	toolsFlag, err := cmd.Flags().GetString("tools")
	if err != nil {
		// Try root persistent flags
		toolsFlag, err = cmd.Root().PersistentFlags().GetString("tools")
	}

	if err == nil && toolsFlag != "" {
		// Ensure --toolsets is not also set
		toolsetsFlag, _ := cmd.Flags().GetString("toolsets")
		if toolsetsFlag == "" {
			toolsetsFlag, _ = cmd.Root().PersistentFlags().GetString("toolsets")
		}
		if toolsetsFlag != "" && toolsetsFlag != "default" {
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
			toolsetsFlag = "default"
		}
	}
	return parseToolsets(toolsetsFlag, logger)
}

func initLogger(outPath string, level log.Level) (*log.Logger, error) {
	logger := log.New()
	logger.SetLevel(level)

	if outPath == "" {
		return logger, nil
	}

	file, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger.SetOutput(file)

	return logger, nil
}

// getLogLevel determines the log level from environment variable or CLI flag
func getLogLevel(cmd *cobra.Command) log.Level {
	// Check environment variable first
	if envLevel := os.Getenv("stdio_LEVEL"); envLevel != "" {
		level, err := log.ParseLevel(envLevel)
		if err != nil {
			stdlog.Printf("Warning: %v, using default 'info' level\n", err)
			return log.InfoLevel
		}
		return level
	}

	// Check CLI flag
	if cmd != nil {
		flagLevel, err := cmd.Flags().GetString("log-level")
		if err == nil && flagLevel != "" {
			level, err := log.ParseLevel(flagLevel)
			if err != nil {
				stdlog.Printf("Warning: %v, using default 'info' level\n", err)
				return log.InfoLevel
			}
			return level
		}
	}

	// Default to info level
	return log.InfoLevel
}

// shouldUseStreamableHTTPMode checks if environment variables indicate HTTP mode
func shouldUseStreamableHTTPMode() bool {
	transportMode := os.Getenv("TRANSPORT_MODE")
	log.Infof("Checking if streamable HTTP mode should be used with TRANSPORT_MODE=%s", transportMode)
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
