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

package root

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/file"
	"oras.land/oras/cmd/oras/internal/display/status"
)

func validateZIPName(name string) error {
	name = strings.TrimSuffix(name, "/")
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("unsafe archive entry name %q", name)
	}
	return nil
}

type pullZIP struct {
	w     *zip.Writer
	names map[string]bool // true for directories
}

func (z *pullZIP) add(name string, mode fs.FileMode, r io.Reader) error {
	if err := validateZIPName(name); err != nil {
		return err
	}
	if !mode.IsDir() && strings.HasSuffix(name, "/") {
		return fmt.Errorf("file entry %q has a directory name", name)
	}
	name = strings.TrimSuffix(name, "/")
	if directory, ok := z.names[name]; ok {
		if directory && mode.IsDir() {
			return nil
		}
		return fmt.Errorf("duplicate archive entry %q", name)
	}
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if directory, ok := z.names[parent]; ok && !directory {
			return fmt.Errorf("archive entry %q is below a file", name)
		}
		z.names[parent] = true
	}
	z.names[name] = mode.IsDir()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
	if mode.IsDir() {
		header.Name += "/"
		header.Method = zip.Store
	}
	header.SetMode(mode)
	w, err := z.w.CreateHeader(header)
	if err != nil || mode.IsDir() {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

func writePullZIP(ctx context.Context, w io.Writer, src content.Fetcher, files []ocispec.Descriptor, handler status.PullHandler) (err error) {
	z := &pullZIP{w: zip.NewWriter(w), names: make(map[string]bool)}
	defer func() { err = errors.Join(err, z.w.Close()) }()
	for _, desc := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := handler.OnNodeDownloading(desc); err != nil {
			return err
		}
		if err := z.copy(ctx, src, desc); err != nil {
			return fmt.Errorf("archive %q: %w", desc.Annotations[ocispec.AnnotationTitle], err)
		}
		if err := handler.OnNodeDownloaded(desc); err != nil {
			return err
		}
	}
	return nil
}

func (z *pullZIP) copy(ctx context.Context, src content.Fetcher, desc ocispec.Descriptor) (err error) {
	rc, err := src.Fetch(ctx, desc)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rc.Close()) }()
	r := content.NewVerifyReader(rc, desc)
	name := desc.Annotations[ocispec.AnnotationTitle]
	unpack := desc.Annotations[file.AnnotationUnpack]
	legacy := desc.Annotations[file.AnnotationUnpackLegacy] //nolint:staticcheck // Accept archives produced before oras-go v3.
	if unpack != "" && legacy != "" && (unpack == "true") != (legacy == "true") {
		return errors.New("conflicting directory unpack annotations")
	}
	if unpack == "" {
		unpack = legacy
	}
	if unpack == "true" {
		err = z.directory(name, desc.Annotations, r)
	} else {
		err = z.add(name, 0644, r)
	}
	if err != nil {
		return err
	}
	return r.Verify()
}

func (z *pullZIP) directory(name string, annotations map[string]string, r io.Reader) (err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, gz.Close()) }()
	var unpacked io.Reader = gz
	checksum := annotations[file.AnnotationDigest]
	legacy := annotations[file.AnnotationDigestLegacy] //nolint:staticcheck // Accept archives produced before oras-go v3.
	if checksum != "" && legacy != "" && checksum != legacy {
		return errors.New("conflicting directory digest annotations")
	}
	if checksum == "" {
		checksum = legacy
	}
	var verifier digest.Verifier
	if checksum != "" {
		d := digest.Digest(checksum)
		if err := d.Validate(); err != nil {
			return err
		}
		verifier = d.Verifier()
		unpacked = io.TeeReader(gz, verifier)
	}
	tr := tar.NewReader(unpacked)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entry := strings.TrimSuffix(h.Name, "/")
		if entry != name && !strings.HasPrefix(entry, name+"/") {
			return fmt.Errorf("directory entry %q is outside %q", h.Name, name)
		}
		mode := fs.FileMode(0644)
		switch h.Typeflag {
		case tar.TypeDir:
			mode = fs.ModeDir | 0755
		case tar.TypeReg:
			if h.Mode&0111 != 0 {
				mode = 0755
			}
		default:
			return fmt.Errorf("unsupported directory entry type %d for %q: ZIP output supports regular files and directories", h.Typeflag, h.Name)
		}
		if err := z.add(entry, mode, tr); err != nil {
			return err
		}
	}
	// Read TAR padding and the gzip trailer before verifying either digest.
	if _, err := io.Copy(io.Discard, unpacked); err != nil {
		return err
	}
	if verifier != nil && !verifier.Verified() {
		return errors.New("directory content digest mismatch")
	}
	return nil
}
