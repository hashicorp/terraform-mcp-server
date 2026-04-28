package main

import (
	"fmt"
	stdlog "log"
	"os"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:     "terraform-mcp-server",
		Short:   "Terraform MCP Server",
		Long:    `A Terraform MCP server that handles various tools and resources.`,
		Version: fmt.Sprintf("Version: %s\nCommit: %s\nBuild Date: %s", version.GetHumanVersion(), version.GitCommit, version.BuildDate),
		Run:     runDefaultCommand,
	}

	stdioCmd = &cobra.Command{
		Use:   "stdio",
		Short: "Start stdio server",
		Long:  `Start a server that communicates via standard input/output streams using JSON-RPC messages.`,
		Run: func(cmd *cobra.Command, _ []string) {
			logFile, err := rootCmd.PersistentFlags().GetString("log-file")
			if err != nil {
				stdlog.Fatal("Failed to get log file:", err)
			}
			logLevel := getLogLevel(cmd.Root())
			logger, err := initLogger(logFile, logLevel)
			if err != nil {
				stdlog.Fatal("Failed to initialize logger:", err)
			}

			enabledToolsets := getToolsetsFromCmd(cmd.Root(), logger)

			if err := runStdioServer(logger, enabledToolsets); err != nil {
				stdlog.Fatal("failed to run stdio server:", err)
			}
		},
	}

	streamableHTTPCmd = &cobra.Command{
		Use:   "streamable-http",
		Short: "Start StreamableHTTP server",
		Long:  `Start a server that communicates via StreamableHTTP transport on port 8080 at /mcp endpoint.`,
		Run: func(cmd *cobra.Command, _ []string) {
			logFile, err := rootCmd.PersistentFlags().GetString("log-file")
			if err != nil {
				stdlog.Fatal("Failed to get log file:", err)
			}
			logLevel := getLogLevel(cmd.Root())
			logger, err := initLogger(logFile, logLevel)
			if err != nil {
				stdlog.Fatal("Failed to initialize logger:", err)
			}

			port, err := cmd.Flags().GetString("transport-port")
			if err != nil {
				stdlog.Fatal("Failed to get streamableHTTP port:", err)
			}
			host, err := cmd.Flags().GetString("transport-host")
			if err != nil {
				stdlog.Fatal("Failed to get streamableHTTP host:", err)
			}

			endpointPath, err := cmd.Flags().GetString("mcp-endpoint")
			if err != nil {
				stdlog.Fatal("Failed to get endpoint path:", err)
			}

			heartbeatInterval, err := cmd.Flags().GetDuration("heartbeat-interval")
			if err != nil {
				stdlog.Fatal("Failed to get heartbeat-interval:", err)
			}

			enabledToolsets := getToolsetsFromCmd(cmd.Root(), logger)
			stdlog.Printf("Starting StreamableHTTP server with host: %s, port: %s, endpoint: %s, heartbeatInterval: %v, enabledToolsets: %v", host, port, endpointPath, heartbeatInterval, enabledToolsets)

			if err := runHTTPServer(logger, host, port, endpointPath, heartbeatInterval, enabledToolsets); err != nil {
				stdlog.Fatal("failed to run streamableHTTP server:", err)
			}
		},
	}
)

func main() {

}

// runDefaultCommand handles the default behavior when no subcommand is provided
func runDefaultCommand(cmd *cobra.Command, _ []string) {
	// Default to stdio mode when no subcommand is provided
	logFile, err := cmd.PersistentFlags().GetString("log-file")
	if err != nil {
		stdlog.Fatal("Failed to get log file:", err)
	}
	logLevel := getLogLevel(cmd)
	logger, err := initLogger(logFile, logLevel)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}

	// Get toolsets from the command that was passed in
	enabledToolsets := getToolsetsFromCmd(cmd, logger)

	if err := runStdioServer(logger, enabledToolsets); err != nil {
		stdlog.Fatal("failed to run stdio server:", err)
	}
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
