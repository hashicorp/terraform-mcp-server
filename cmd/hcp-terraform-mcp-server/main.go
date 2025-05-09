// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"hcp-terraform-mcp-server/pkg/hashicorp"
	"hcp-terraform-mcp-server/pkg/hashicorp/tfenterprise"
	"hcp-terraform-mcp-server/pkg/hashicorp/tfregistry"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	// TODO: Refactor dependent packages to use TFE client instead of GitHub client

	iolog "github.com/github/github-mcp-server/pkg/log"

	// gogithub "github.com/google/go-github/v69/github" // Removed GitHub client import

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "version"
var commit = "commit"
var date = "date"

func InitRegistryClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	return client
}

var (
	rootCmd = &cobra.Command{
		Use:     "server",
		Short:   "HCP Terraform MCP Server",
		Long:    `A HCP Terraform MCP server that handles various tools and resources.`,
		Version: fmt.Sprintf("Version: %s\nCommit: %s\nBuild Date: %s", version, commit, date),
	}

	stdioCmd = &cobra.Command{
		Use:   "stdio",
		Short: "Start stdio server",
		Long:  `Start a server that communicates via standard input/output streams using JSON-RPC messages.`,
		Run: func(_ *cobra.Command, _ []string) {
			logFile := viper.GetString("log-file")
			readOnly := viper.GetBool("read-only")
			logger, err := initLogger(logFile)
			if err != nil {
				stdlog.Fatal("Failed to initialize logger:", err)
			}

			enabledToolsets := viper.GetStringSlice("toolsets")

			logCommands := viper.GetBool("enable-command-logging")
			cfg := runConfig{
				readOnly:        readOnly,
				logger:          logger,
				logCommands:     logCommands,
				enabledToolsets: enabledToolsets,
			}
			if err := runStdioServer(cfg); err != nil {
				stdlog.Fatal("failed to run stdio server:", err)
			}
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")

	// Add global flags that will be shared by all commands
	rootCmd.PersistentFlags().StringSlice("toolsets", tfregistry.DefaultTools, "An optional comma separated list of groups of tools to allow, defaults to enabling all")
	rootCmd.PersistentFlags().Bool("dynamic-toolsets", false, "Enable dynamic toolsets")
	rootCmd.PersistentFlags().Bool("read-only", false, "Restrict the server to read-only operations")
	rootCmd.PersistentFlags().String("log-file", "", "Path to log file")
	rootCmd.PersistentFlags().Bool("enable-command-logging", false, "When enabled, the server will log all command requests and responses to the log file")
	rootCmd.PersistentFlags().Bool("export-translations", false, "Save translations to a JSON file")

	// Bind flag to viper
	_ = viper.BindPFlag("toolsets", rootCmd.PersistentFlags().Lookup("toolsets"))
	_ = viper.BindPFlag("dynamic_toolsets", rootCmd.PersistentFlags().Lookup("dynamic-toolsets"))
	_ = viper.BindPFlag("read-only", rootCmd.PersistentFlags().Lookup("read-only"))
	_ = viper.BindPFlag("log-file", rootCmd.PersistentFlags().Lookup("log-file"))
	_ = viper.BindPFlag("enable-command-logging", rootCmd.PersistentFlags().Lookup("enable-command-logging"))
	_ = viper.BindPFlag("export-translations", rootCmd.PersistentFlags().Lookup("export-translations"))

	// Add subcommands
	rootCmd.AddCommand(stdioCmd)
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

type runConfig struct {
	readOnly        bool
	logger          *log.Logger
	logCommands     bool
	enabledToolsets []string
}

func runStdioServer(cfg runConfig) error {
	var analytics hashicorp.Analytics
	cfg.logger.Info("initializing analytics")
	analytics = hashicorp.NewSegmentAnalytics(
		"<INSERT SEGMENT KEY>", cfg.logger,
	)

	analytics.Track("mcp_server_started", map[string]interface{}{
		"version": version,
		"commit":  commit,
		"date":    date,
	})

	defer analytics.Close()
	// Create app context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hcServer := hashicorp.NewServer(version)

	tfeToken := viper.GetString("HCP_TFE_TOKEN")
	if tfeToken != "" {
		tfeAddress := viper.GetString("HCP_TFE_ADDRESS") // Example: "https://app.terraform.io"
		if tfeAddress == "" {
			tfeAddress = "https://app.terraform.io"
			cfg.logger.Warnf("HCP_TFE_ADDRESS not set, defaulting to %s", tfeAddress)
		}
		tfenterprise.Init(hcServer, tfeToken, tfeAddress)
	} else {
		cfg.logger.Warnf("HCP_TFE_TOKEN not set, defaulting to non-authenticated client")
	}

	registryClient := InitRegistryClient()
	tfregistry.InitTools(hcServer, registryClient, analytics, cfg.logger)
	tfregistry.RegisterResources(hcServer, registryClient, cfg.logger)
	tfregistry.RegisterResourceTemplates(hcServer, registryClient, cfg.logger)

	stdioServer := server.NewStdioServer(hcServer)
	stdLogger := stdlog.New(cfg.logger.Writer(), "stdioserver", 0)
	stdioServer.SetErrorLogger(stdLogger)

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)

		if cfg.logCommands {
			loggedIO := iolog.NewIOLogger(in, out, cfg.logger)
			in, out = loggedIO, loggedIO
		}

		errC <- stdioServer.Listen(ctx, in, out)
	}()

	// Output github-mcp-server string // TODO: Update this message?
	_, _ = fmt.Fprintf(os.Stderr, "HCP Terraform MCP Server running on stdio\n")

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		cfg.logger.Infof("shutting down server...")
	case err := <-errC:
		if err != nil {
			return fmt.Errorf("error running server: %w", err)
		}
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
