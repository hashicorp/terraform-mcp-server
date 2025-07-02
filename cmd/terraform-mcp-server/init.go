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

	"github.com/hashicorp/terraform-mcp-server/pkg/hashicorp/tfregistry"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func InitRegistryClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	return client
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")
	rootCmd.PersistentFlags().String("log-file", "", "Path to log file")

	// Add HTTP command flags (avoid 'h' shorthand conflict with help)
	httpCmd.Flags().String("transport-host", "0.0.0.0", "Host to bind to")
	httpCmd.Flags().StringP("transport-port", "p", "8080", "Port to listen on")

	rootCmd.AddCommand(stdioCmd)
	rootCmd.AddCommand(httpCmd)
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
	registryClient := InitRegistryClient()
	tfregistry.InitTools(hcServer, registryClient, logger)
	tfregistry.RegisterResources(hcServer, registryClient, logger)
	tfregistry.RegisterResourceTemplates(hcServer, registryClient, logger)
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
