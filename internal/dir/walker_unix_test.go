//go:build unix

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

package dir

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestWalk_RejectsFIFO verifies that unsupported filesystem node types fail the
// walk explicitly instead of being typed as regular files. Opening a FIFO
// without a writer during loading would block indefinitely.
func TestWalk_RejectsFIFO(t *testing.T) {
	tmpDir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(tmpDir, "pipe"), 0644); err != nil {
		t.Skipf("mkfifo not supported: %v", err)
	}

	_, err := Walk(tmpDir, WalkOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("got error %v, want unsupported file type error", err)
	}
}
