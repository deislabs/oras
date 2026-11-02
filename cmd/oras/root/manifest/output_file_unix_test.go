//go:build !windows

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
	"syscall"
	"testing"
)

func Test_writeOutputFile_newFileHonorsUmask(t *testing.T) {
	// not parallel: umask is process wide
	old := syscall.Umask(0077)
	defer syscall.Umask(old)

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := writeOutputFile(path, writeString("x")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func Test_writeOutputFile_symlinkIsKept(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(target, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.json", link); err != nil {
		t.Fatal(err)
	}
	if err := writeOutputFile(link, writeString("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a regular file")
	}
	if got := readFile(t, target); got != "new" {
		t.Errorf("target content = %q", got)
	}
	tinfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := tinfo.Mode().Perm(); got != 0640 {
		t.Errorf("target mode = %o, want 640", got)
	}
	assertNoTempFiles(t, dir, 2)
}

func Test_writeOutputFile_fifo(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}

	got := make(chan string, 1)
	go func() {
		// opening for read unblocks the writer's open
		f, err := os.Open(fifo)
		if err != nil {
			got <- "open error: " + err.Error()
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		got <- string(b)
	}()

	if err := writeOutputFile(fifo, writeString("through fifo")); err != nil {
		t.Fatal(err)
	}
	if s := <-got; s != "through fifo" {
		t.Errorf("fifo received %q", s)
	}
	info, err := os.Lstat(fifo)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Error("FIFO was replaced")
	}
	assertNoTempFiles(t, dir, 1)
}

func Test_writeOutputFile_charDevice(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err != nil {
		t.Skip("/dev/null not available")
	}
	if err := writeOutputFile("/dev/null", writeString("discarded")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat("/dev/null"); err != nil || info.Mode().IsRegular() {
		t.Errorf("/dev/null was damaged: %v, %v", info, err)
	}
}

func Test_writeOutputFile_symlinkFailureKeepsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.json", link); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("digest mismatch")
	err := writeOutputFile(link, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got := readFile(t, target); got != "original" {
		t.Errorf("target content = %q, want original", got)
	}
	assertNoTempFiles(t, dir, 2)
}

func Test_writeOutputFile_danglingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink("target.json", link); err != nil {
		t.Fatal(err)
	}
	if err := writeOutputFile(link, writeString("new")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "new" {
		t.Errorf("target content = %q", got)
	}
	assertNoTempFiles(t, dir, 2)
}

func Test_writeOutputFile_readOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	if err := writeOutputFile(path, writeString("new")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new" {
		t.Errorf("content = %q", got)
	}
	assertNoTempFiles(t, dir, 1)
}
