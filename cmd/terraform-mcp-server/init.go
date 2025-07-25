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
	"strconv"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-mcp-server/pkg/resources"
	"github.com/hashicorp/terraform-mcp-server/pkg/tools"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func InitRegistryClient(logger *log.Logger) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = logger

	transport := cleanhttp.DefaultPooledTransport()
	transport.Proxy = http.ProxyFromEnvironment

	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.HTTPClient.Timeout = 10 * time.Second
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = 3

	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			resetAfterInt, err := strconv.ParseInt(resetAfter, 10, 64)
			if err != nil {
				return 0
			}
			resetAfterTime := time.Unix(resetAfterInt, 0)
			return time.Until(resetAfterTime)
		}
		return 0
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			return resetAfter != "", nil
		}
		return false, nil
	}

	return retryClient.StandardClient()
}

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

func registryInit(hcServer *server.MCPServer, logger *log.Logger) {
	registryClient := InitRegistryClient(logger)
	tools.InitTools(hcServer, registryClient, logger)
	resources.RegisterResources(hcServer, registryClient, logger)
	resources.RegisterResourceTemplates(hcServer, registryClient, logger)
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
