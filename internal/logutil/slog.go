/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package logutil provides utilities for bridging loggers.
package logutil

import (
	"context"
	"log/slog"

	"github.com/sirupsen/logrus"
)

// NewSlogLogger returns a slog.Logger that forwards records to logger.
func NewSlogLogger(logger logrus.FieldLogger) *slog.Logger {
	return slog.New(&handler{logger: logger})
}

// handler is a slog.Handler that forwards records to a logrus.FieldLogger.
type handler struct {
	logger logrus.FieldLogger
	prefix string
}

// Enabled reports whether the underlying logger handles records at level.
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	var logger *logrus.Logger
	switch l := h.logger.(type) {
	case *logrus.Logger:
		logger = l
	case *logrus.Entry:
		logger = l.Logger
	default:
		return true
	}
	return logger.IsLevelEnabled(toLogrusLevel(level))
}

// Handle forwards the record to the underlying logger.
func (h *handler) Handle(_ context.Context, record slog.Record) error {
	fields := logrus.Fields{}
	record.Attrs(func(attr slog.Attr) bool {
		addField(fields, h.prefix, attr)
		return true
	})
	logger := h.logger.WithFields(fields)
	switch toLogrusLevel(record.Level) {
	case logrus.ErrorLevel:
		logger.Error(record.Message)
	case logrus.WarnLevel:
		logger.Warn(record.Message)
	case logrus.InfoLevel:
		logger.Info(record.Message)
	default:
		logger.Debug(record.Message)
	}
	return nil
}

// WithAttrs returns a handler with attrs added as fields.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := logrus.Fields{}
	for _, attr := range attrs {
		addField(fields, h.prefix, attr)
	}
	return &handler{logger: h.logger.WithFields(fields), prefix: h.prefix}
}

// WithGroup returns a handler that prefixes subsequent field keys with name.
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{logger: h.logger, prefix: h.prefix + name + "."}
}

// addField adds attr to fields, flattening groups into prefixed keys.
func addField(fields logrus.Fields, prefix string, attr slog.Attr) {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			prefix += attr.Key + "."
		}
		for _, a := range value.Group() {
			addField(fields, prefix, a)
		}
		return
	}
	if attr.Key == "" {
		return
	}
	fields[prefix+attr.Key] = value.Any()
}

// toLogrusLevel maps a slog level to the corresponding logrus level.
func toLogrusLevel(level slog.Level) logrus.Level {
	switch {
	case level >= slog.LevelError:
		return logrus.ErrorLevel
	case level >= slog.LevelWarn:
		return logrus.WarnLevel
	case level >= slog.LevelInfo:
		return logrus.InfoLevel
	default:
		return logrus.DebugLevel
	}
}
