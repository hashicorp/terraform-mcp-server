// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevelFromCommand(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		flagValue   string
		expected    log.Level
		description string
	}{
		{
			name:        "env var takes precedence",
			envValue:    "debug",
			flagValue:   "error",
			expected:    log.DebugLevel,
			description: "LOG_LEVEL env var should override --log-level flag",
		},
		{
			name:        "flag used when env not set",
			envValue:    "",
			flagValue:   "warn",
			expected:    log.WarnLevel,
			description: "--log-level flag should be used when LOG_LEVEL is not set",
		},
		{
			name:        "default when neither set",
			envValue:    "",
			flagValue:   "",
			expected:    log.InfoLevel,
			description: "should default to info level when neither env nor flag is set",
		},
		{
			name:        "invalid env falls back to default",
			envValue:    "invalid",
			flagValue:   "",
			expected:    log.InfoLevel,
			description: "invalid LOG_LEVEL should fall back to default info level",
		},
		{
			name:        "invalid flag falls back to default",
			envValue:    "",
			flagValue:   "invalid",
			expected:    log.InfoLevel,
			description: "invalid --log-level should fall back to default info level",
		},
		{
			name:        "env overrides even with invalid flag",
			envValue:    "error",
			flagValue:   "invalid",
			expected:    log.ErrorLevel,
			description: "valid LOG_LEVEL should be used even if flag is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := os.Getenv("LOG_LEVEL")
			defer func() {
				if originalEnv != "" {
					os.Setenv("LOG_LEVEL", originalEnv)
				} else {
					os.Unsetenv("LOG_LEVEL")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("log-level", tt.flagValue, "test flag")

			level := LevelFromCommand(cmd)
			if level != tt.expected {
				t.Errorf("%s: expected level %v, got %v", tt.description, tt.expected, level)
			}
		})
	}
}

func TestLevelFromCommandWithNilCommand(t *testing.T) {
	originalEnv := os.Getenv("LOG_LEVEL")
	defer func() {
		if originalEnv != "" {
			os.Setenv("LOG_LEVEL", originalEnv)
		} else {
			os.Unsetenv("LOG_LEVEL")
		}
	}()

	os.Unsetenv("LOG_LEVEL")
	level := LevelFromCommand(nil)
	if level != log.InfoLevel {
		t.Errorf("expected default info level with nil command, got %v", level)
	}

	os.Setenv("LOG_LEVEL", "debug")
	level = LevelFromCommand(nil)
	if level != log.DebugLevel {
		t.Errorf("expected debug level from env var with nil command, got %v", level)
	}
}

func TestSlogLevelFromCommand(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		flagValue   string
		expected    slog.Level
		description string
	}{
		{
			name:        "env var takes precedence",
			envValue:    "debug",
			flagValue:   "error",
			expected:    slog.LevelDebug,
			description: "when both are set, LOG_LEVEL=debug should take precedence over --log-level=error",
		},
		{
			name:        "flag used when env not set",
			envValue:    "",
			flagValue:   "warn",
			expected:    slog.LevelWarn,
			description: "when LOG_LEVEL is unset, --log-level=warn should select the warn level",
		},
		{
			name:        "default when neither set",
			envValue:    "",
			flagValue:   "",
			expected:    slog.LevelInfo,
			description: "when neither LOG_LEVEL nor --log-level is set, the level should default to info",
		},
		{
			name:        "trace maps to debug",
			envValue:    "trace",
			flagValue:   "",
			expected:    slog.LevelDebug,
			description: "because slog has no trace level, LOG_LEVEL=trace should use the debug level",
		},
		{
			name:        "level is case insensitive and trimmed",
			envValue:    " WARN ",
			flagValue:   "",
			expected:    slog.LevelWarn,
			description: "LOG_LEVEL should ignore letter case and surrounding whitespace, so ' WARN ' should select warn",
		},
		{
			name:        "fatal maps to error",
			envValue:    "fatal",
			flagValue:   "",
			expected:    slog.LevelError,
			description: "because slog has no fatal level, LOG_LEVEL=fatal should use the error level",
		},
		{
			name:        "panic maps to error",
			envValue:    "panic",
			flagValue:   "",
			expected:    slog.LevelError,
			description: "because slog has no panic level, LOG_LEVEL=panic should use the error level",
		},
		{
			name:        "invalid env falls back to default",
			envValue:    "invalid",
			flagValue:   "",
			expected:    slog.LevelInfo,
			description: "when LOG_LEVEL contains an unsupported value, the level should fall back to info",
		},
		{
			name:        "invalid flag falls back to default",
			envValue:    "",
			flagValue:   "invalid",
			expected:    slog.LevelInfo,
			description: "when LOG_LEVEL is unset and --log-level is unsupported, the level should fall back to info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := os.Getenv("LOG_LEVEL")
			defer func() {
				if originalEnv != "" {
					os.Setenv("LOG_LEVEL", originalEnv)
				} else {
					os.Unsetenv("LOG_LEVEL")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("log-level", tt.flagValue, "test flag")

			level := SlogLevelFromCommand(cmd)
			if level != tt.expected {
				t.Errorf("%s: expected level %v, got %v", tt.description, tt.expected, level)
			}
		})
	}
}

func TestSlogLevelFromCommandWithNilCommand(t *testing.T) {
	originalEnv := os.Getenv("LOG_LEVEL")
	defer func() {
		if originalEnv != "" {
			os.Setenv("LOG_LEVEL", originalEnv)
		} else {
			os.Unsetenv("LOG_LEVEL")
		}
	}()

	os.Unsetenv("LOG_LEVEL")
	level := SlogLevelFromCommand(nil)
	if level != slog.LevelInfo {
		t.Errorf("expected default info level with nil command, got %v", level)
	}

	os.Setenv("LOG_LEVEL", "debug")
	level = SlogLevelFromCommand(nil)
	if level != slog.LevelDebug {
		t.Errorf("expected debug level from env var with nil command, got %v", level)
	}
}

func TestNewLoggerWithLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    log.Level
		expected log.Level
	}{
		{"trace level", log.TraceLevel, log.TraceLevel},
		{"debug level", log.DebugLevel, log.DebugLevel},
		{"info level", log.InfoLevel, log.InfoLevel},
		{"warn level", log.WarnLevel, log.WarnLevel},
		{"error level", log.ErrorLevel, log.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger("", tt.level, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if logger.GetLevel() != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, logger.GetLevel())
			}
		})
	}
}

func TestNewLoggerWithFormat(t *testing.T) {
	tests := []struct {
		name              string
		logFormat         string
		expectedLogFormat log.Formatter
	}{
		{"empty format", "", &log.TextFormatter{}},
		{"text format", "text", &log.TextFormatter{}},
		{"json format", "json", &log.JSONFormatter{}},
		{"TEXT format", "TEXT", &log.TextFormatter{}},
		{"JSON format", "JSON", &log.JSONFormatter{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger("", log.InfoLevel, tt.logFormat)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tt.expectedLogFormat.(type) {
			case *log.JSONFormatter:
				if _, ok := logger.Formatter.(*log.JSONFormatter); !ok {
					t.Errorf("expected JSONFormatter, got %T", logger.Formatter)
				}
			case *log.TextFormatter:
				if _, ok := logger.Formatter.(*log.TextFormatter); !ok {
					t.Errorf("expected TextFormatter, got %T", logger.Formatter)
				}
			}
		})
	}
}

func TestNewSlogLoggerWithLevel(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
	}{
		{"debug level", slog.LevelDebug},
		{"info level", slog.LevelInfo},
		{"warn level", slog.LevelWarn},
		{"error level", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "official.log")
			logger, logFile, err := NewSlogLogger(logPath, tt.level, "")
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, logFile.Close())
			})

			assert.True(t, logger.Enabled(context.Background(), tt.level))
			assert.False(t, logger.Enabled(context.Background(), tt.level-4))
		})
	}
}

func TestNewSlogLoggerWithFormat(t *testing.T) {
	tests := []struct {
		name      string
		logFormat string
		wantJSON  bool
	}{
		{"empty format", "", false},
		{"text format", "text", false},
		{"json format", "json", true},
		{"TEXT format", "TEXT", false},
		{"JSON format", "JSON", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "official.log")
			logger, logFile, err := NewSlogLogger(logPath, slog.LevelInfo, tt.logFormat)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, logFile.Close())
			})

			if tt.wantJSON {
				_, ok := logger.Handler().(*slog.JSONHandler)
				assert.True(t, ok, "expected JSONHandler, got %T", logger.Handler())
			} else {
				_, ok := logger.Handler().(*slog.TextHandler)
				assert.True(t, ok, "expected TextHandler, got %T", logger.Handler())
			}
		})
	}
}

func TestFormatFromCommand(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		flagValue   string
		expected    string
		description string
	}{
		{
			name:        "env var takes precedence",
			envValue:    "json",
			flagValue:   "text",
			expected:    "json",
			description: "LOG_FORMAT env var should override --log-format flag",
		},
		{
			name:        "flag used when env not set",
			envValue:    "",
			flagValue:   "json",
			expected:    "json",
			description: "--log-format flag should be used when LOG_FORMAT is not set",
		},
		{
			name:        "default when neither set",
			envValue:    "",
			flagValue:   "",
			expected:    "text",
			description: "should default to text format when neither env nor flag is set",
		},
		{
			name:        "invalid env falls back to default",
			envValue:    "invalid",
			flagValue:   "",
			expected:    "text",
			description: "invalid LOG_FORMAT should fall back to default text format",
		},
		{
			name:        "invalid flag falls back to default",
			envValue:    "",
			flagValue:   "invalid",
			expected:    "text",
			description: "invalid --log-format should fall back to default text format",
		},
		{
			name:        "env overrides even with invalid flag",
			envValue:    "json",
			flagValue:   "invalid",
			expected:    "json",
			description: "valid LOG_FORMAT should be used even if flag is invalid",
		},
		{
			name:        "case insensitive - uppercase JSON",
			envValue:    "JSON",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should be case insensitive (JSON -> json)",
		},
		{
			name:        "case insensitive - uppercase TEXT",
			envValue:    "TEXT",
			flagValue:   "",
			expected:    "text",
			description: "LOG_FORMAT should be case insensitive (TEXT -> text)",
		},
		{
			name:        "case insensitive - mixed case Json",
			envValue:    "Json",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should be case insensitive (Json -> json)",
		},
		{
			name:        "whitespace trimmed from env",
			envValue:    "  json  ",
			flagValue:   "",
			expected:    "json",
			description: "LOG_FORMAT should trim whitespace from env value",
		},
		{
			name:        "whitespace trimmed from flag",
			envValue:    "",
			flagValue:   "  text  ",
			expected:    "text",
			description: "--log-format should trim whitespace from flag value",
		},
		{
			name:        "text format explicitly set via env",
			envValue:    "text",
			flagValue:   "",
			expected:    "text",
			description: "text format should be explicitly settable via LOG_FORMAT",
		},
		{
			name:        "text format explicitly set via flag",
			envValue:    "",
			flagValue:   "text",
			expected:    "text",
			description: "text format should be explicitly settable via --log-format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := os.Getenv("LOG_FORMAT")
			defer func() {
				if originalEnv != "" {
					os.Setenv("LOG_FORMAT", originalEnv)
				} else {
					os.Unsetenv("LOG_FORMAT")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("LOG_FORMAT", tt.envValue)
			} else {
				os.Unsetenv("LOG_FORMAT")
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("log-format", tt.flagValue, "test flag")

			format := FormatFromCommand(cmd)
			if format != tt.expected {
				t.Errorf("%s: expected format %q, got %q", tt.description, tt.expected, format)
			}
		})
	}
}

func TestFormatFromCommandWithNilCommand(t *testing.T) {
	originalEnv := os.Getenv("LOG_FORMAT")
	defer func() {
		if originalEnv != "" {
			os.Setenv("LOG_FORMAT", originalEnv)
		} else {
			os.Unsetenv("LOG_FORMAT")
		}
	}()

	os.Unsetenv("LOG_FORMAT")
	format := FormatFromCommand(nil)
	if format != "text" {
		t.Errorf("expected default text format with nil command, got %q", format)
	}

	os.Setenv("LOG_FORMAT", "json")
	format = FormatFromCommand(nil)
	if format != "json" {
		t.Errorf("expected json format from env var with nil command, got %q", format)
	}

	os.Setenv("LOG_FORMAT", "text")
	format = FormatFromCommand(nil)
	if format != "text" {
		t.Errorf("expected text format from env var with nil command, got %q", format)
	}

	os.Setenv("LOG_FORMAT", "invalid")
	format = FormatFromCommand(nil)
	if format != "text" {
		t.Errorf("expected default text format with invalid env var and nil command, got %q", format)
	}
}
