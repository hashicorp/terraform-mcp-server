// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/resources"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools"
	"github.com/hashicorp/terraform-mcp-server/version"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		Run: func(_ *cobra.Command, _ []string) {
			logFile, err := rootCmd.PersistentFlags().GetString("log-file")
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
			logger, err := initLogger(logFile)
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

			if err := runHTTPServer(logger, host, port, endpointPath); err != nil {
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

	// Add StreamableHTTP command flags (avoid 'h' shorthand conflict with help)
	streamableHTTPCmd.Flags().String("transport-host", "127.0.0.1", "Host to bind to")
	streamableHTTPCmd.Flags().StringP("transport-port", "p", "8080", "Port to listen on")
	streamableHTTPCmd.Flags().String("mcp-endpoint", "/mcp", "Path for streamable HTTP endpoint")

	// Add the same flags to the alias command for backward compatibility
	httpCmdAlias.Flags().String("transport-host", "127.0.0.1", "Host to bind to")
	httpCmdAlias.Flags().StringP("transport-port", "p", "8080", "Port to listen on")
	httpCmdAlias.Flags().String("mcp-endpoint", "/mcp", "Path for streamable HTTP endpoint")

	rootCmd.AddCommand(stdioCmd)
	rootCmd.AddCommand(streamableHTTPCmd)
	rootCmd.AddCommand(httpCmdAlias) // Add the alias for backward compatibility
}

func initConfig() {
	viper.AutomaticEnv()
}

func initLogger(outPath string) (*log.Logger, error) {
	if outPath == "" {
		return log.New(), nil
	}

	file, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New()
	logger.SetLevel(log.DebugLevel)
	logger.SetOutput(file)

	return logger, nil
}

// registerToolsAndResources registers tools and resources with the MCP server
func registerToolsAndResources(hcServer *server.MCPServer, logger *log.Logger) {
	tools.RegisterTools(hcServer, logger)
	resources.RegisterResources(hcServer, logger)
	resources.RegisterResourceTemplates(hcServer, logger)
}

func serverInit(ctx context.Context, hcServer *server.MCPServer, logger *log.Logger) error {
	stdioServer := server.NewStdioServer(hcServer)
	stdLogger := stdlog.New(logger.Writer(), "stdioserver", 0)
	stdioServer.SetErrorLogger(stdLogger)

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)
		errC <- stdioServer.Listen(ctx, in, out)
	}()

	_, _ = fmt.Fprintf(os.Stderr, "Terraform MCP Server running on stdio\n")

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		logger.Infof("shutting down server...")
	case err := <-errC:
		if err != nil {
			return fmt.Errorf("error running server: %w", err)
		}
	}

	return nil
}

func streamableHTTPServerInit(ctx context.Context, hcServer *server.MCPServer, logger *log.Logger, host string, port string, endpointPath string) error {
	// Ensure endpoint path starts with /
	endpointPath = path.Join("/", endpointPath)
	// Create StreamableHTTP server which implements the new streamable-http transport
	// This is the modern MCP transport that supports both direct HTTP responses and SSE streams
	opts := []server.StreamableHTTPOption{
		server.WithEndpointPath(endpointPath), // Default MCP endpoint path
		server.WithLogger(logger),
	}

	// Log the endpoint path being used
	logger.Infof("Using endpoint path: %s", endpointPath)

	// Check if stateless mode is enabled
	isStateless := shouldUseStatelessMode()
	opts = append(opts, server.WithStateLess(isStateless))
	logger.Infof("Running with stateless mode: %v", isStateless)

	baseStreamableServer := server.NewStreamableHTTPServer(hcServer, opts...)

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
	streamableServer := client.NewSecurityHandler(baseStreamableServer, corsConfig.AllowedOrigins, corsConfig.Mode, logger)

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
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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
