package main

import (
	"context"
	"fmt"
	"hcp-terraform-mcp-server/pkg/hashicorp"
	"hcp-terraform-mcp-server/pkg/hashicorp/tfenterprise"
	"hcp-terraform-mcp-server/pkg/hashicorp/tfregistry"
	"io"
	stdlog "log"
	"os"
	"os/signal"
	"syscall"

	// TODO: Refactor dependent packages to use TFE client instead of GitHub client

	iolog "github.com/github/github-mcp-server/pkg/log"
	"github.com/github/github-mcp-server/pkg/translations"

	// gogithub "github.com/google/go-github/v69/github" // Removed GitHub client import

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "version"
var commit = "commit"
var date = "date"

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
			exportTranslations := viper.GetBool("export-translations")
			logger, err := initLogger(logFile)
			if err != nil {
				stdlog.Fatal("Failed to initialize logger:", err)
			}

			enabledToolsets := viper.GetStringSlice("toolsets")

			logCommands := viper.GetBool("enable-command-logging")
			cfg := runConfig{
				readOnly:           readOnly,
				logger:             logger,
				logCommands:        logCommands,
				exportTranslations: exportTranslations,
				enabledToolsets:    enabledToolsets,
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
	readOnly           bool
	logger             *log.Logger
	logCommands        bool
	exportTranslations bool
	enabledToolsets    []string
}

func runStdioServer(cfg runConfig) error {
	// Create app context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	t, dumpTranslations := translations.TranslationHelper()
	enabled := cfg.enabledToolsets
	dynamic := viper.GetBool("dynamic_toolsets")
	if dynamic {
		// filter "all" from the enabled toolsets
		enabled = make([]string, 0, len(cfg.enabledToolsets))
		for _, toolset := range cfg.enabledToolsets {
			if toolset != "all" {
				enabled = append(enabled, toolset)
			}
		}
	}
	hcServer := hashicorp.NewServer(version)

	tfeToken := viper.GetString("HCP_TFE_TOKEN")
	if tfeToken != "" {
		tfeAddress := viper.GetString("HCP_TFE_ADDRESS") // Example: "https://app.terraform.io"
		if tfeAddress == "" {
			tfeAddress = "https://app.terraform.io"
			cfg.logger.Warnf("HCP_TFE_ADDRESS not set, defaulting to %s", tfeAddress)
		}
		tfenterprise.Init(hcServer, tfeToken, tfeAddress, enabled, cfg.readOnly, t)
	} else {
		cfg.logger.Warnf("HCP_TFE_TOKEN not set, defaulting to non-authenticated client")
	}

	// Initialize default service discovery and http client for registry
	// discoClient := disco.New() // Restore disco client initialization
	// httpClient := http.DefaultClient
	// registryClient := registry.NewClient(discoClient, httpClient) // Restore registry client initialization

	// Initialize toolsets that are used for TF Registry - no auth is needed
	// toolsets, err := tfregistry.InitToolsets(enabled, cfg.readOnly, registryClient, t) // Restore toolset initialization
	// context := tfregistry.InitContextToolset(registryClient, t)                        // Restore context initialization

	// if err != nil { // Restore error check
	// 	stdlog.Fatal("Failed to initialize toolsets:", err) // This error check might need adjustment based on refactoring
	// } // Restore error check

	// // Register resources with the server
	// tfregistry.RegisterResources(hcServer, registryClient, t) // Restore resource registration
	// // Register the tools with the server
	// toolsets.RegisterTools(hcServer) // Restore tool registration
	// context.RegisterTools(hcServer)  // Restore context registration

	// if dynamic {
	// 	dynamic := tfregistry.InitDynamicToolset(hcServer, toolsets, t) // Restore dynamic toolset initialization
	// 	dynamic.RegisterTools(hcServer)                                 // Restore dynamic tool registration
	// }

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

	if cfg.exportTranslations {
		// Once server is initialized, all translations are loaded
		dumpTranslations()
	}

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
