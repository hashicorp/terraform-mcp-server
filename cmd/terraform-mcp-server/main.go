// Copyright IBM Corp. 2025
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
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//go:embed instructions.md
var instructions string
var sessionClientInfo sync.Map // map[string]client.ClientInfo

func runHTTPServer(logger *log.Logger, host string, port string, endpointPath string, heartbeatInterval time.Duration, enabledToolsets []string, metricsConfig client.MetricsConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create hooks for session management
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		client.NewSessionHandler(ctx, session, logger)
	})
	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		// Clean up client info populated in the metrics hooks, for the session
		sessionClientInfo.Delete(session.SessionID())
		client.EndSessionHandler(ctx, session, logger)
	})
	// When running multiple sessions of the MCP server (load balancing), calling client.NewSessionHandler
	// in both BeforeListTools and BeforeCallTool ensures that a session that was not initialized during
	// registration (e.g., due to being routed to a different instance) will still have its clients created
	// before any tool calls are made. This provides a safety net to ensure that all sessions have
	// the necessary clients initialized regardless of how they are routed.
	hooks.AddBeforeListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest) {
		session := server.ClientSessionFromContext(ctx)
		if session != nil {
			client.NewSessionHandler(ctx, session, logger)
		}
	})
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		session := server.ClientSessionFromContext(ctx)
		if session != nil {
			client.NewSessionHandler(ctx, session, logger)
		}
	})
	attachMetricsHooks(hooks, metricsConfig, logger)

	opts := []server.ServerOption{server.WithHooks(hooks)}

	hcServer := NewServer(version.Version, logger, enabledToolsets, opts...)
	registerToolsAndResources(hcServer, logger, enabledToolsets)

	return streamableHTTPServerInit(ctx, hcServer, logger, host, port, endpointPath, heartbeatInterval)
}

func attachMetricsHooks(hooks *server.Hooks, metricsConfig client.MetricsConfig, logger *log.Logger) {
	if !metricsConfig.Enabled {
		return
	}
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		if message != nil && message.Params.ClientInfo.Name != "" {
			session := server.ClientSessionFromContext(ctx)
			if session == nil {
				logger.Debug("AddAfterInitialize hook: No session found in context")
				return
			}
			ci := client.ClientInfo{
				Name:        message.Params.ClientInfo.Name,
				Version:     message.Params.ClientInfo.Version,
				Title:       message.Params.ClientInfo.Title,
				Description: message.Params.ClientInfo.Description,
			}
			// Record the client info in the session first so we can reuse it in the BeforeToolCall hook
			sessionClientInfo.Store(session.SessionID(), ci)
			// Record the metric
			client.RecordClientType(ctx, ci, metricsConfig, logger)
		}
	})

	var toolStartTimes sync.Map
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		toolStartTimes.Store(fmt.Sprintf("%v", id), time.Now())
		session := server.ClientSessionFromContext(ctx)
		if session == nil {
			logger.Debug("AddBeforeCallTool hook: No session found in context")
			return
		}
		value, ok := sessionClientInfo.Load(session.SessionID())
		if !ok {
			logger.Debugf("AddBeforeCallTool hook: Client info not found for session ID: %s", session.SessionID())
			return
		}
		// Read the client info recorded in the AddAfterInitialize hook
		info, ok := value.(client.ClientInfo)
		if !ok || info.Name == "" {
			logger.Debugf("AddBeforeCallTool hook: Unable to read client info for sessionID %s from sessionClientInfo map", session.SessionID())
			return
		}
		client.RecordClientType(
			ctx,
			info,
			metricsConfig,
			logger,
		)
	})
	hooks.AddAfterCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest, result any) {
		startTime := time.Now()
		if storedStart, ok := toolStartTimes.LoadAndDelete(fmt.Sprintf("%v", id)); ok {
			if ts, ok := storedStart.(time.Time); ok {
				startTime = ts
			}
		}

		var toolErr bool
		if res, ok := result.(*mcp.CallToolResult); ok && res.IsError {
			toolErr = true
		}
		client.RecordToolCall(ctx, startTime, toolErr, id, message, metricsConfig, logger)
	})
}

func runStdioServer(logger *log.Logger, enabledToolsets []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create hooks for session management
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		client.NewSessionHandler(ctx, session, logger)
	})
	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		client.EndSessionHandler(ctx, session, logger)
	})

	hcServer := NewServer(version.Version, logger, enabledToolsets, server.WithHooks(hooks))
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

// runDefaultCommand handles the default behavior when no subcommand is provided
func runDefaultCommand(cmd *cobra.Command, _ []string) {
	// Default to stdio mode when no subcommand is provided
	logFile, err := cmd.PersistentFlags().GetString("log-file")
	if err != nil {
		stdlog.Fatal("Failed to get log file:", err)
	}
	logLevel := getLogLevel(cmd)
	logFormat := getLogFormat(cmd)
	logger, err := initLogger(logFile, logLevel, logFormat)
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
	logFile, _ := rootCmd.PersistentFlags().GetString("log-file")
	logLevel := getLogLevel(rootCmd)
	logFormat := getLogFormat(rootCmd)
	logger, err := initLogger(logFile, logLevel, logFormat)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}
	if shouldUseStreamableHTTPMode() {
		logger.Info("Starting in Streamable HTTP mode based on environment configuration")

		metricsConfig, shutdownMetrics := setupMetrics(logger)
		defer shutdownMetrics()

		port := getHTTPPort()
		host := getHTTPHost()
		endpointPath := getEndpointPath(nil)
		enabledToolsets := getToolsetsFromCmd(rootCmd, logger)
		heartbeatInterval := getHeartbeatInterval()
		if err := runHTTPServer(logger, host, port, endpointPath, heartbeatInterval, enabledToolsets, metricsConfig); err != nil {
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

func setupMetrics(logger *log.Logger) (client.MetricsConfig, func()) {
	metricsConfig := client.LoadMetricsConfigFromEnv()
	logger.Infof("Metrics enabled: %t endpoint: %s exportInterval: %s", metricsConfig.Enabled, metricsConfig.Endpoint, metricsConfig.ExportInterval)
	if !metricsConfig.Enabled {
		return metricsConfig, func() {}
	}

	// Context for metrics is for tracking the lifecycle of the metrics setup and shutdown, not tied to individual tool calls.
	ctxMetrics := context.Background()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Errorf("OTel Internal Error: %v", err)
	}))

	shutdown, err := initMetrics(ctxMetrics, &metricsConfig, logger)
	if err != nil {
		logger.Errorf("Failed to initialize metrics: %v", err)
		return metricsConfig, func() {}
	}

	return metricsConfig, shutdown
}

func initMetrics(ctx context.Context, config *client.MetricsConfig, logger *log.Logger) (func(), error) {
	logger.Infof("Initializing exporter and meter provider for OTel metrics...")
	// Create the Exporter (Sends data to the Collector)
	// exporter, err := stdoutmetric.New() // For stdio debugging
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(config.Endpoint), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}
	// Create the MeterProvider with a PeriodicReader
	// The reader flushes metrics to the exporter periodically
	resourceAttrs := resource.NewSchemaless(
		attribute.String("service.name", config.ServiceName),
		attribute.String("service.version", config.ServiceVersion),
	)
	config.MeterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(config.ExportInterval))),
		sdkmetric.WithResource(resourceAttrs),
	)

	// Set it as the global provider
	otel.SetMeterProvider(config.MeterProvider)

	meter := config.MeterProvider.Meter(config.ServiceName)

	config.ToolCounter, err = meter.Int64Counter("mcp_tool_calls_total")
	if err != nil {
		return nil, fmt.Errorf("failed to create tool counter: %w", err)
	}

	config.ErrorCounter, err = meter.Int64Counter("mcp_tool_errors_total",
		metric.WithDescription("Total number of failed tool calls"))
	if err != nil {
		return nil, fmt.Errorf("failed to create error counter: %w", err)
	}

	config.ToolCallLatencyBucket, err = meter.Float64Histogram("mcp_tool_duration_seconds",
		metric.WithDescription("Duration of tool calls in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("failed to create latency histogram: %w", err)
	}

	config.ClientTypeCounter, err = meter.Int64Counter("mcp_client_type_total",
		metric.WithDescription("Total number of connections by client type"))
	if err != nil {
		return nil, fmt.Errorf("failed to create client type counter: %w", err)
	}

	return func() {
		logger.Infof("Shutting down metrics exporter..")
		if err := config.MeterProvider.Shutdown(ctx); err != nil {
			logger.Errorf("Error shutting down meter provider: %v", err)
		}
	}, nil
}
