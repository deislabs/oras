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

package logutil

import (
	"context"
	"log/slog"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestNewSlogLogger_levels(t *testing.T) {
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)
	slogger := NewSlogLogger(logger)

	tests := []struct {
		log  func(string, ...any)
		want logrus.Level
	}{
		{slogger.Debug, logrus.DebugLevel},
		{slogger.Info, logrus.InfoLevel},
		{slogger.Warn, logrus.WarnLevel},
		{slogger.Error, logrus.ErrorLevel},
	}
	for _, tt := range tests {
		hook.Reset()
		tt.log("message")
		entry := hook.LastEntry()
		if entry == nil {
			t.Fatalf("no entry logged for level %v", tt.want)
		}
		if entry.Level != tt.want {
			t.Errorf("level = %v, want %v", entry.Level, tt.want)
		}
		if entry.Message != "message" {
			t.Errorf("message = %q, want %q", entry.Message, "message")
		}
	}
}

func TestNewSlogLogger_enabled(t *testing.T) {
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.WarnLevel)
	slogger := NewSlogLogger(logrus.NewEntry(logger))

	if slogger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info level should be disabled")
	}
	if !slogger.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("warn level should be enabled")
	}
	slogger.Info("dropped")
	if len(hook.AllEntries()) != 0 {
		t.Errorf("got %d entries, want 0", len(hook.AllEntries()))
	}
}

func TestNewSlogLogger_fields(t *testing.T) {
	logger, hook := test.NewNullLogger()
	entry := logger.WithField("existing", "value")
	slogger := NewSlogLogger(entry).With("with", 1).WithGroup("group").With("inner", true)

	slogger.Warn("warning", "registry", "localhost:5000", slog.Group("sub", "key", "v"))
	got := hook.LastEntry()
	if got == nil {
		t.Fatal("no entry logged")
	}
	want := logrus.Fields{
		"existing":       "value",
		"with":           int64(1),
		"group.inner":    true,
		"group.registry": "localhost:5000",
		"group.sub.key":  "v",
	}
	if len(got.Data) != len(want) {
		t.Errorf("fields = %v, want %v", got.Data, want)
	}
	for k, v := range want {
		if got.Data[k] != v {
			t.Errorf("field %q = %v, want %v", k, got.Data[k], v)
		}
	}
}

func TestNewSlogLogger_registryAttr(t *testing.T) {
	logger, hook := test.NewNullLogger()
	NewSlogLogger(logger).Warn("warning", "registry", "localhost:5000")
	got := hook.LastEntry()
	if got == nil {
		t.Fatal("no entry logged")
	}
	if got.Data["registry"] != "localhost:5000" {
		t.Errorf("registry = %v, want %v", got.Data["registry"], "localhost:5000")
	}
}
