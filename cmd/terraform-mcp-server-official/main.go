package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	mcpofficial "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	logFile, _ := rootCmd.PersistentFlags().GetString("log-file")
	logLevel := getLogLevel(rootCmd)
	logger, err := initLogger(logFile, logLevel)
	if err != nil {
		stdlog.Fatal("Failed to initialize logger:", err)
	}
	if shouldUseStreamableHTTPMode() {
		logger.Info("Starting in Streamable HTTP mode based on environment configuration")
		port := getHTTPPort()
		host := getHTTPHost()
		endpointPath := getEndpointPath(nil)
		enabledToolsets := getToolsetsFromCmd(rootCmd, logger)
		heartbeatInterval := getHeartbeatInterval()
		if err := runHTTPServer(logger, host, port, endpointPath, heartbeatInterval, enabledToolsets); err != nil {
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

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")
	rootCmd.PersistentFlags().String("log-file", "", "Path to log file")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().String("log-format", "text", "Log format (text or json)")
	rootCmd.PersistentFlags().String("toolsets", "all", toolsets.GenerateToolsetsHelp())
	rootCmd.PersistentFlags().String("tools", "", toolsets.GenerateToolsHelp())

	// Add StreamableHTTP command flags (avoid 'h' shorthand conflict with help)
	streamableHTTPCmd.Flags().String("transport-host", "127.0.0.1", "Host to bind to")
	streamableHTTPCmd.Flags().StringP("transport-port", "p", "8080", "Port to listen on")
	streamableHTTPCmd.Flags().Duration("heartbeat-interval", 0, "Heartbeat interval for HTTP connections (e.g., 30s). 0 to disable")
	streamableHTTPCmd.Flags().String("mcp-endpoint", "/mcp", "Path for streamable HTTP endpoint")

	rootCmd.AddCommand(stdioCmd)
	rootCmd.AddCommand(streamableHTTPCmd)
}

func initConfig() {
	viper.AutomaticEnv()
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

func runStdioServer(logger *log.Logger, enabledToolsets []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hcServer := mcpofficial.NewServer(0)
	if err := hcServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Stdio server error: %v", err)
		return err
	}
	return nil
}

func runHTTPServer(logger *log.Logger, host, port, endpointPath string, heartbeatInterval time.Duration, enabledToolsets []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hcServer := mcpofficial.NewServer(heartbeatInterval)

	// Ensure endpoint path starts with /
	endpointPath = path.Join("/", endpointPath)
	logger.Infof("Using endpoint path: %s", endpointPath)

	var handler http.Handler

	// Check if stateless mode is enabled
	isStateless := shouldUseStatelessMode()
	logger.Infof("Running with stateless mode: %v", isStateless)

	// Create StreamableHTTP server which implements the new streamable-http transport
	// This is the modern MCP transport that supports both direct HTTP responses and SSE streams
	opts := &mcp.StreamableHTTPOptions{
		Stateless: isStateless,
	}
	// Load TLS configuration
	tlsConfig, err := client.GetTLSConfigFromEnv()
	if err != nil {
		return fmt.Errorf("TLS configuration error: %w", err)
	}

	// Create the base MCP handler
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return hcServer
	}, opts)

	// Load CORS configuration
	corsConfig := client.LoadCORSConfigFromEnv()
	// Log CORS configuration
	logger.Infof("CORS Mode: %s", corsConfig.Mode)
	if len(corsConfig.AllowedOrigins) > 0 {
		logger.Infof("Allowed Origins: %s", strings.Join(corsConfig.AllowedOrigins, ", "))
	} else if corsConfig.Mode == "strict" {
		logger.Warnf("No allowed origins configured in strict mode. All cross-origin requests will be rejected.")
	} else if corsConfig.Mode == "development" {
		logger.Infof("Development mode: localhost origins are automatically allowed")
	} else if corsConfig.Mode == "disabled" {
		logger.Warnf("CORS validation is disabled. This is not recommended for production.")
	}

	// Create a security wrapper around the streamable server
	streamableServer := client.NewSecurityHandler(mcpHandler, corsConfig.AllowedOrigins, corsConfig.Mode, logger)

	mux := http.NewServeMux()

	// Apply middleware
	streamableServer = client.TerraformContextMiddleware(logger)(streamableServer)

	// Handle the /mcp endpoint with the streamable server (with security wrapper)
	mux.Handle(endpointPath, streamableServer)
	mux.Handle(endpointPath+"/", streamableServer)

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`{"status":"ok","service":"terraform-mcp-server","transport":"streamable-http","endpoint":"%s"}`, endpointPath)
		w.Write([]byte(response))
	})

	addr := fmt.Sprintf("%s:%s", host, port)
	if enableOtelMetrics := os.Getenv("OTEL_METRICS_ENABLED"); enableOtelMetrics == "true" {
		// Add http server instrumentation for standard server metrics
		handler = otelhttp.NewHandler(mux, "terraform-mcp-server")
	} else {
		handler = mux
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if tlsConfig != nil {
		httpServer.TLSConfig = tlsConfig.Config
		logger.Infof("TLS enabled with certificate: %s", tlsConfig.CertFile)
	} else {
		if !client.IsLocalHost(host) {
			return fmt.Errorf("TLS is required for non-localhost binding (%s). Set MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE environment variables", host)
		}
		logger.Warnf("TLS is disabled on StreamableHTTP server; this is not recommended for production")
	}

	// Start server in goroutine
	errC := make(chan error, 1)
	go func() {
		logger.Infof("Starting StreamableHTTP server on %s%s", addr, endpointPath)
		errC <- httpServer.ListenAndServe()
	}()

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		logger.Infof("Shutting down StreamableHTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errC:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("StreamableHTTP server error: %w", err)
		}
	}

	return nil
}
