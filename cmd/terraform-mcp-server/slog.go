// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log/slog"
	"strings"

	log "github.com/sirupsen/logrus"
)

// define a concrete object for slog.Handler interface
type logrusSlogHandler struct {
	logger *log.Logger
	fields log.Fields
	groups []string
}

// newSlogLogger wraps a Logrus logger in a *slog.Logger
func newSlogLogger(logger *log.Logger) *slog.Logger {
	return slog.New(&logrusSlogHandler{
		logger: logger,
		fields: make(log.Fields),
	})
}

// -- slog.Handler interface implemented by log.logger (logrus) --------------------------------------------------

// reports whether the handler handles records at the given level.
func (h *logrusSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.IsLevelEnabled(slogLevelToLogrus(level))
}

func (h *logrusSlogHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make(log.Fields, len(h.fields)+record.NumAttrs())
	// field copy h.fields
	for key, value := range h.fields {
		fields[key] = value
	}

	// field copy record attrs
	record.Attrs(func(attr slog.Attr) bool {
		h.addAttr(fields, attr)
		return true
	})

	entry := h.logger.WithFields(fields)

	if !record.Time.IsZero() {
		entry = entry.WithTime(record.Time)
	}

	entry.Log(slogLevelToLogrus(record.Level), record.Message)

	return nil
}

func (h *logrusSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handler := &logrusSlogHandler{
		logger: h.logger,
		fields: make(log.Fields, len(h.fields)+len(attrs)),
		groups: append([]string(nil), h.groups...),
	}

	for key, value := range h.fields {
		handler.fields[key] = value
	}

	for _, attr := range attrs {
		handler.addAttr(handler.fields, attr)
	}

	return handler
}

func (h *logrusSlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	fields := make(log.Fields, len(h.fields))
	for key, value := range h.fields {
		fields[key] = value
	}

	return &logrusSlogHandler{
		logger: h.logger,
		fields: fields,
		groups: append(append([]string(nil), h.groups...), name),
	}
}

// -- helpers -----------------------------------------------------------------

// addAttr flattens a slog.Attr into the Logrus fields map.
// Group attributes are expanded with dot-separated key prefixes.
func (h *logrusSlogHandler) addAttr(fields log.Fields, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()

	if attr.Value.Kind() == slog.KindGroup {
		groups := h.groups
		if attr.Key != "" {
			groups = append(append([]string(nil), groups...), attr.Key)
		}

		child := &logrusSlogHandler{
			logger: h.logger,
			groups: groups,
		}

		for _, nested := range attr.Value.Group() {
			child.addAttr(fields, nested)
		}

		return
	}

	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(
			append(append([]string(nil), h.groups...), key),
			".",
		)
	}

	fields[key] = attr.Value.Any()
}

// slogLevelToLogrus converts a slog.Level to the nearest Logrus log.Level.
func slogLevelToLogrus(slogLevel slog.Level) log.Level {
	switch {
	case slogLevel >= slog.LevelError:
		return log.ErrorLevel
	case slogLevel >= slog.LevelWarn:
		return log.WarnLevel
	case slogLevel >= slog.LevelInfo:
		return log.InfoLevel
	default:
		return log.DebugLevel
	}
}
