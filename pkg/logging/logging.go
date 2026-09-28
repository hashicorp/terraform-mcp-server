// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// LevelFromCommand determines the logrus log level from the LOG_LEVEL
// environment variable or the --log-level flag on cmd
func LevelFromCommand(cmd *cobra.Command) log.Level {
	// Check environment variable first
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
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

// SlogLevelFromCommand determines the slog level from the LOG_LEVEL
// environment variable or the --log-level flag on cmd
func SlogLevelFromCommand(cmd *cobra.Command) slog.Level {
	configuredLevel := os.Getenv("LOG_LEVEL")
	if configuredLevel == "" && cmd != nil {
		flagLevel, err := cmd.Flags().GetString("log-level")
		if err == nil {
			configuredLevel = flagLevel
		}
	}

	switch strings.ToLower(strings.TrimSpace(configuredLevel)) {
	case "trace", "debug":
		return slog.LevelDebug
	case "", "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error", "fatal", "panic":
		return slog.LevelError
	default:
		stdlog.Printf("Warning: invalid slog level %q, using default 'info' level\n", configuredLevel)
		return slog.LevelInfo
	}
}

// FormatFromCommand determines the log format ("json" or "text") from the
// LOG_FORMAT environment variable or the --log-format flag on cmd.
func FormatFromCommand(cmd *cobra.Command) string {
	// Check environment variable first
	if envFormat := os.Getenv("LOG_FORMAT"); envFormat != "" {
		format := strings.ToLower(strings.TrimSpace(envFormat))
		if format == "json" || format == "text" {
			return format
		}
		stdlog.Printf("Warning: invalid LOG_FORMAT '%s', using default 'text' format\n", envFormat)
		return "text"
	}

	// Check CLI flag
	if cmd != nil {
		if flagFormat, err := cmd.Flags().GetString("log-format"); err == nil && flagFormat != "" {
			format := strings.ToLower(strings.TrimSpace(flagFormat))
			if format == "json" || format == "text" {
				return format
			}
			stdlog.Printf("Warning: invalid --log-format '%s', using default 'text' format\n", flagFormat)
		}
	}

	return "text"
}

// NewLogger builds a logrus logger at the given level and format, writing to
// outPath if set, or stdout otherwise
func NewLogger(outPath string, level log.Level, format string) (*log.Logger, error) {
	logger := log.New()
	logger.SetLevel(level)

	// Set formatter based on format parameter
	if strings.ToLower(format) == "json" {
		logger.SetFormatter(&log.JSONFormatter{})
	} else {
		logger.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}

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

// NewSlogLogger builds an slog logger at the given level and format, writing
// to outPath if set, or stderr otherwise. The returned file is nil when
// outPath is empty; callers are responsible for closing it otherwise.
func NewSlogLogger(outPath string, level slog.Level, format string) (*slog.Logger, *os.File, error) {
	out := io.Writer(os.Stderr)
	var file *os.File

	if outPath != "" {
		var err error
		file, err = os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %w", err)
		}
		out = file
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}
	var handler slog.Handler
	// Set formatter based on format parameter
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(out, opts)
	} else {
		handler = slog.NewTextHandler(out, opts)
	}
	return slog.New(handler), file, nil
}
