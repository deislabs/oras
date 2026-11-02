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

package manifest

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeString(s string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoTempFiles(t *testing.T, dir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != want {
		t.Errorf("directory has %d entries, want %d: %v", len(entries), want, entries)
	}
}

func Test_writeOutputFile_newFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := writeOutputFile(path, writeString("hello")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "hello" {
		t.Errorf("content = %q", got)
	}
	assertNoTempFiles(t, dir, 1)
}

func Test_writeOutputFile_overwriteRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := os.WriteFile(path, []byte("old content that is longer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeOutputFile(path, writeString("new")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new" {
		t.Errorf("content = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("mode = %o, want 600", got)
		}
	}
	assertNoTempFiles(t, dir, 1)
}

func Test_writeOutputFile_failureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("boom")
	err := writeOutputFile(path, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got := readFile(t, path); got != "original" {
		t.Errorf("content = %q, want original", got)
	}
	assertNoTempFiles(t, dir, 1)
}

func Test_writeOutputFile_longBasename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, strings.Repeat("a", 250))
	if err := writeOutputFile(path, writeString("long")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "long" {
		t.Errorf("content = %q", got)
	}
	assertNoTempFiles(t, dir, 1)
}
