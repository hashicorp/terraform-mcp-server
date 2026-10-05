// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"
	stdlog "log"

	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	mcpmark3labs "github.com/hashicorp/terraform-mcp-server/pkg/mcp-mark3labs"
	"github.com/hashicorp/terraform-mcp-server/pkg/otelmetrics"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rootCmd = &cobra.Command{
		Use:     "terraform-mcp-server",
		Short:   "Terraform MCP Server",
		Long:    `A Terraform MCP server that handles various tools and resources.`,
		Version: fmt.Sprintf("Version: %s", version.GetHumanVersion()),
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
			logLevel := logging.LevelFromCommand(cmd.Root())
			logFormat := logging.FormatFromCommand(cmd)
			logger, err := logging.NewLogger(logFile, logLevel, logFormat)
			if err != nil {
				stdlog.Fatal("Failed to initialize logger:", err)
			}

			enabledToolsets := getToolsetsFromCmd(cmd.Root(), logger)

			if err := mcpmark3labs.RunStdioServer(logger, enabledToolsets); err != nil {
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
			logLevel := logging.LevelFromCommand(cmd.Root())
			logFormat := logging.FormatFromCommand(cmd)
			logger, err := logging.NewLogger(logFile, logLevel, logFormat)
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
			organizationAllowlist, err := getOrganizationAllowlist(cmd)
			if err != nil {
				stdlog.Fatal(err)
			}
			logger.Printf("Starting StreamableHTTP server with host: %s, port: %s, endpoint: %s, heartbeatInterval: %v, enabledToolsets: %v, organizationAllowlistConfigured: %t, organizationAllowlistCount: %d", host, port, endpointPath, heartbeatInterval, enabledToolsets, len(organizationAllowlist) > 0, len(organizationAllowlist))
			metricsConfig, shutdownMetrics := otelmetrics.Setup(logger)
			defer shutdownMetrics()

			if err := mcpmark3labs.RunHTTPServer(logger, host, port, endpointPath, heartbeatInterval, enabledToolsets, metricsConfig, organizationAllowlist, rootCmd); err != nil {
				stdlog.Fatal("failed to run streamableHTTP server:", err)
			}
		},
	}

	// Create an alias for backward compatibility
	httpCmdAlias = &cobra.Command{
		Use:        "http",
		Short:      "Start StreamableHTTP server (deprecated, use 'streamable-http' instead)",
		Long:       `This command is deprecated. Please use 'streamable-http' instead.`,
		Deprecated: "Use 'streamable-http' instead",
		Run: func(cmd *cobra.Command, args []string) {
			// Forward to the new command
			streamableHTTPCmd.Run(cmd, args)
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")
	rootCmd.PersistentFlags().String("log-file", "", "Path to log file")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().String("log-format", "text", "Log format (text or json)")
	rootCmd.PersistentFlags().String("toolsets", toolsets.All, toolsets.GenerateToolsetsHelp())
	rootCmd.PersistentFlags().String("tools", "", toolsets.GenerateToolsHelp())

	// Add StreamableHTTP command flags (avoid 'h' shorthand conflict with help)
	streamableHTTPCmd.Flags().String("transport-host", "127.0.0.1", "Host to bind to")
	streamableHTTPCmd.Flags().StringP("transport-port", "p", "8080", "Port to listen on")
	streamableHTTPCmd.Flags().Duration("heartbeat-interval", 0, "Heartbeat interval for HTTP connections (e.g., 30s). 0 to disable")
	streamableHTTPCmd.Flags().String("mcp-endpoint", "/mcp", "Path for streamable HTTP endpoint")
	streamableHTTPCmd.Flags().String("organization-allowlist", "", "Comma-separated list of HCP Terraform organization names allowed to access the HTTP server")

	// Add the same flags to the alias command for backward compatibility
	httpCmdAlias.Flags().String("transport-host", "127.0.0.1", "Host to bind to")
	httpCmdAlias.Flags().StringP("transport-port", "p", "8080", "Port to listen on")
	httpCmdAlias.Flags().String("mcp-endpoint", "/mcp", "Path for streamable HTTP endpoint")
	httpCmdAlias.Flags().Duration("heartbeat-interval", 0, "Heartbeat interval for HTTP connections (e.g., 30s). 0 to disable")
	httpCmdAlias.Flags().String("organization-allowlist", "", "Comma-separated list of HCP Terraform organization names allowed to access the HTTP server")

	rootCmd.AddCommand(stdioCmd)
	rootCmd.AddCommand(streamableHTTPCmd)
	rootCmd.AddCommand(httpCmdAlias) // Add the alias for backward compatibility
}

func initConfig() {
	viper.AutomaticEnv()
}
