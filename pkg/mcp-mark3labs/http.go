// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package mcpmark3labs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/instana"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/version"
	instanasdk "github.com/instana/go-sensor"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Transport string `json:"transport"`
	Endpoint  string `json:"endpoint"`
	Version   string `json:"version"`
}

var sessionClientInfo sync.Map // map[string]client.ClientInfo

// RunHTTPServer builds the mark3labs MCP server and serves it over
// StreamableHTTP, mounting the official go-sdk bridge alongside it when
// enabled, until the process receives a shutdown signal. rootCmd is needed
// to resolve the official go-sdk logger's level/format/file when the bridge
// is enabled.
func RunHTTPServer(logger *log.Logger, host string, port string, endpointPath string, heartbeatInterval time.Duration, filter toolsets.ToolFilter, metricsConfig client.MetricsConfig, organizationAllowlist []string, rootCmd *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create hooks for session management
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		client.NewSessionHandler(ctx, session.SessionID(), logger)
	})

	serverOpts := []server.ServerOption{
		server.WithHooks(hooks),
	}

	// Only register the middleware when an organization allowlist is configured
	if len(organizationAllowlist) > 0 {
		serverOpts = append(serverOpts, server.WithToolHandlerMiddleware(
			client.OrganizationAllowlistToolMiddleware(organizationAllowlist, logger),
		))
	}
	hcServer, rateLimiter := NewServer(
		version.Version,
		logger,
		filter,
		serverOpts...,
	)

	registerToolsAndResources(hcServer, logger, filter)

	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		// Clean up client info populated in the metrics hooks, for the session
		sessionClientInfo.Delete(session.SessionID())
		client.EndSessionHandler(ctx, session.SessionID(), rateLimiter, logger)
	})
	// When running multiple sessions of the MCP server (load balancing), calling client.NewSessionHandler
	// in both BeforeListTools and BeforeCallTool ensures that a session that was not initialized during
	// registration (e.g., due to being routed to a different instance) will still have its clients created
	// before any tool calls are made. This provides a safety net to ensure that all sessions have
	// the necessary clients initialized regardless of how they are routed.
	hooks.AddBeforeListTools(func(ctx context.Context, id any, message *mcp.ListToolsRequest) {
		session := server.ClientSessionFromContext(ctx)
		if session != nil {
			client.NewSessionHandler(ctx, session.SessionID(), logger)
		}
	})
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		session := server.ClientSessionFromContext(ctx)
		if session != nil {
			client.NewSessionHandler(ctx, session.SessionID(), logger)
		}
	})
	attachMetricsHooks(hooks, metricsConfig, logger)

	return streamableHTTPServerInit(ctx, hcServer, logger, host, port, endpointPath, heartbeatInterval, organizationAllowlist, filter, rateLimiter, metricsConfig, rootCmd)
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

func streamableHTTPServerInit(ctx context.Context, hcServer *server.MCPServer, logger *log.Logger, host string, port string, endpointPath string, heartbeatInterval time.Duration, organizationAllowlist []string, filter toolsets.ToolFilter, rateLimiter *client.RateLimitMiddleware, metricsConfig client.MetricsConfig, rootCmd *cobra.Command) error {
	// Ensure endpoint path starts with /
	endpointPath = path.Join("/", endpointPath)
	var handler http.Handler

	// Initialize the Instana collector if enabled (nil when disabled).
	instanaCollector := instana.Setup(logger)

	// Create StreamableHTTP server which implements the new streamable-http transport
	// This is the modern MCP transport that supports both direct HTTP responses and SSE streams
	opts := []server.StreamableHTTPOption{
		server.WithEndpointPath(endpointPath), // Default MCP endpoint path
		server.WithStreamableHTTPLogger(logging.WrapLogrus(logger)),
	}

	// Load TLS configuration
	tlsConfig, err := client.GetTLSConfigFromEnv()
	if err != nil {
		return fmt.Errorf("TLS configuration error: %w", err)
	}
	if tlsConfig != nil {
		opts = append(opts, server.WithTLSCert(tlsConfig.CertFile, tlsConfig.KeyFile))
	}

	// Log the endpoint path being used
	logger.Infof("Using endpoint path: %s", endpointPath)

	// Check if stateless mode is enabled
	isStateless := shouldUseStatelessMode()
	opts = append(opts, server.WithStateLess(isStateless))
	logger.Infof("Running with stateless mode: %v", isStateless)

	// Configure heartbeat interval if enabled
	if heartbeatInterval > 0 {
		opts = append(opts, server.WithHeartbeatInterval(heartbeatInterval))
		logger.Infof("HTTP heartbeat enabled with interval: %v", heartbeatInterval)
	}

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

	mux := http.NewServeMux()

	// Apply middleware
	streamableServer := client.OrganizationAllowlistMiddleware(organizationAllowlist, logger)(baseStreamableServer)
	streamableServer = client.TerraformContextMiddleware(logger)(streamableServer)
	streamableServer = client.NewSecurityHandler(streamableServer, corsConfig.AllowedOrigins, corsConfig.Mode, logger)

	// Handle the /mcp endpoint with the streamable server (with security wrapper)
	mux.Handle(endpointPath, streamableServer)
	mux.Handle(endpointPath+"/", streamableServer)

	// Create the official go-sdk streamable server
	if enableOfficialSDK := os.Getenv("TF_X_OFFICIAL_SDK_ENABLED"); enableOfficialSDK == "true" {
		logger.Info("TF_X_OFFICIAL_SDK_ENABLED set to true in env, enabling the official mcp go-sdk server")
		logFile, err := rootCmd.PersistentFlags().GetString("log-file")
		if err != nil {
			return fmt.Errorf("failed to get log file: %w", err)
		}
		slogLevel := logging.SlogLevelFromCommand(rootCmd)
		slogFormat := logging.FormatFromCommand(rootCmd)
		officialLogger, officialLogFile, err := logging.NewSlogLogger(logFile, slogLevel, slogFormat)
		if err != nil {
			return fmt.Errorf("failed to initialize official MCP slog logger: %w", err)
		}
		if officialLogFile != nil {
			defer func() {
				if err := officialLogFile.Close(); err != nil {
					logger.Errorf("Failed to close official MCP log file at %s: %v", logFile, err)
				}
			}()
		}

		officialLogger = officialLogger.With("component", "mcp-official")
		officialStreamableServer := getOfficialStreamableServer(ctx, heartbeatInterval, isStateless, corsConfig, logger, officialLogger, organizationAllowlist, filter, rateLimiter, metricsConfig)
		// Handle the /mcp endpoint with the official go-sdk streamable server (with security wrapper)
		mux.Handle(endpointPath+"/official", officialStreamableServer)
		mux.Handle(endpointPath+"/official/", officialStreamableServer)
	}

	if redirectURL := os.Getenv("MCP_REDIRECT_ROOT_URL"); redirectURL != "" {
		logger.Infof("Requests to `/` will be redirected to %s", redirectURL)
		// handle root direct if it's configured
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		})
	}

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response, err := json.Marshal(healthResponse{
			Status:    "ok",
			Service:   "terraform-mcp-server",
			Transport: "streamable-http",
			Endpoint:  endpointPath,
			Version:   version.GetHumanVersion(),
		})
		if err != nil {
			logger.Errorf("Failed to marshal health response: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(response)
	})

	addr := fmt.Sprintf("%s:%s", host, port)
	handler = mux
	if enableOtelMetrics := os.Getenv("OTEL_METRICS_ENABLED"); enableOtelMetrics == "true" {
		// Add http server instrumentation for standard server metrics
		handler = otelhttp.NewHandler(handler, "terraform-mcp-server")
	}
	if instanaCollector != nil {
		// Wrapping the handler so incoming HTTP requests will be able to be traced by Instana
		handler = instanasdk.TracingHandlerFunc(instanaCollector, "", handler.ServeHTTP)
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
		if tlsConfig != nil {
			errC <- httpServer.ListenAndServeTLS(tlsConfig.CertFile, tlsConfig.KeyFile)
		} else {
			errC <- httpServer.ListenAndServe()
		}
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
