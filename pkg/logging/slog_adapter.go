// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"io"
	"log/slog"

	log "github.com/sirupsen/logrus"
	logrusslog "github.com/sirupsen/logrus/hooks/slog"
)

// WrapSlog wraps a *slog.Logger in a *logrus.Logger, so code that only
// accepts logrus (like the shared pkg/client helpers) can log through slog.
// It is the reverse of WrapLogrus.
//
// The returned logger writes nothing itself: a hook converts each logrus entry into a slog record and hands it to slogLogger.
func WrapSlog(slogLogger *slog.Logger) *log.Logger {
	logrusLogger := log.New()
	logrusLogger.SetOutput(io.Discard) // logrus output is discarded; the hook forwards entries to slog
	logrusLogger.SetLevel(logrusLevelFor(slogLogger))
	logrusLogger.AddHook(logrusslog.NewHook(slogLogger, &logrusslog.HookOptions{
		LevelMapper: logrusLevelToSlog,
	}))
	return logrusLogger
}

// logrusLevelFor returns the most verbose logrus level that slogLogger keeps.
// slog doesn't expose its minimum enabled level, so we have to probe each level with slogLogger.Enabled to determine it.
func logrusLevelFor(slogLogger *slog.Logger) log.Level {
	level := log.PanicLevel
	for _, l := range log.AllLevels { // ordered from least to most verbose
		if slogLogger.Enabled(context.Background(), logrusLevelToSlog(l)) {
			level = l
		}
	}
	return level
}

func logrusLevelToSlog(level log.Level) slog.Level {
	switch level {
	case log.PanicLevel, log.FatalLevel, log.ErrorLevel:
		return slog.LevelError
	case log.WarnLevel:
		return slog.LevelWarn
	case log.InfoLevel:
		return slog.LevelInfo
	default: // DebugLevel, TraceLevel
		return slog.LevelDebug
	}
}
