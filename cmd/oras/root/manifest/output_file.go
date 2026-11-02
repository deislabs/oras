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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// writeOutputFile writes the content produced by write to the file at path.
//
//   - A symbolic link is resolved and the content is written to its target,
//     so the link itself is kept.
//   - If the (resolved) path is an existing non-regular file (e.g.
//     /dev/stdout, a FIFO or a character device), the content is written
//     through it directly, since replacing it would break the stream.
//   - Otherwise the content is written to a temporary file in the same
//     directory which is then renamed to the target, so that a failed or
//     interrupted write never leaves a partially written file behind. The
//     permission bits of an existing file are preserved; a new file gets
//     0666 filtered by the process umask. If no temporary file can be created
//     (e.g. the directory is not writable), the content is written in place.
func writeOutputFile(path string, write func(io.Writer) error) error {
	target := path
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		info = nil
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			// dangling link or a link that cannot be resolved to a path,
			// e.g. /dev/stdout -> /proc/self/fd/1 -> pipe:[N]
			return writeInPlace(path, write)
		}
		target = resolved
		info, err = os.Stat(target)
		if err != nil {
			return err
		}
	}

	if info != nil && !info.Mode().IsRegular() {
		return writeInPlace(target, write)
	}

	// the kernel applies the umask to the requested 0666 mode
	file, err := createTempFile(filepath.Dir(target))
	if err != nil {
		return writeInPlace(target, write)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(file.Name())
		}
	}()

	if info != nil {
		if err := file.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := write(file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), target); err != nil {
		return err
	}
	committed = true
	return nil
}

// writeInPlace opens path for writing without replacing it and streams the
// content into it.
func writeInPlace(path string, write func(io.Writer) error) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); err == nil {
			err = cerr
		}
	}()
	return write(file)
}

// createTempFile exclusively creates a new file in dir with mode 0666
// (before umask). The name does not depend on the output file name, so it
// cannot exceed the file name length limit.
func createTempFile(dir string) (*os.File, error) {
	for range 10000 {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, ".oras-tmp-"+hex.EncodeToString(b[:]))
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("failed to create a temporary output file")
}
