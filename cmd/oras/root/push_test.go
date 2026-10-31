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
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/content/oci"
	oraserrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
)

func Test_runPush_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test
	opts := &pushOptions{
		Format: option.Format{
			Type: "unknown",
		},
	}
	got := runPush(cmd, opts).Error()
	want := oraserrors.UnsupportedFormatTypeError(opts.Format.Type).Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Test_runPush_force exercises pushing with --force, including the extra-tag
// path that uses the TraversingTarget-wrapped destination.
func Test_runPush_force(t *testing.T) {
	// prepare a file to push
	tempDir := t.TempDir()
	fileName := "hi.txt"
	filePath := filepath.Join(tempDir, fileName)
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}

	// use an OCI image layout as the push destination
	layoutDir := filepath.Join(tempDir, "layout")

	cmd := pushCmd()
	cmd.SetArgs([]string{
		"--oci-layout",
		"--force",
		"--disable-path-validation",
		layoutDir + ":tag1,tag2",
		filePath,
	})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify the artifact and both tags were pushed to the layout
	store, err := oci.New(layoutDir)
	if err != nil {
		t.Fatalf("failed to open pushed layout: %v", err)
	}
	for _, tag := range []string{"tag1", "tag2"} {
		if _, err := store.Resolve(context.Background(), tag); err != nil {
			t.Errorf("expected tag %q to be resolvable, got error: %v", tag, err)
		}
	}
}

func Test_pushCmd_invalidExtraTag(t *testing.T) {
	cmd := pushCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1,,invalid@@tag",
		"hi.txt",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != "empty extra tag" {
		t.Fatalf("got %v, want empty extra tag error", err)
	}
}

func Test_runPushRecursive_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// test
	opts := &pushOptions{
		Format: option.Format{
			Type: "unknown",
		},
		Recursive: option.Recursive{
			Recursive:           true,
			MaxBlobsPerManifest: 1000,
		},
	}
	opts.FileRefs = []string{tmpDir}

	got := runPushRecursive(cmd, opts).Error()
	want := oraserrors.UnsupportedFormatTypeError(opts.Format.Type).Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_runPushRecursive_emptyDir(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	tmpDir := t.TempDir()

	// test with empty directory and no preserve-empty-dirs
	opts := &pushOptions{
		Recursive: option.Recursive{
			Recursive:           true,
			MaxBlobsPerManifest: 1000,
			PreserveEmptyDirs:   false,
		},
	}
	opts.FileRefs = []string{tmpDir}

	err := runPushRecursive(cmd, opts)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if err.Error() != "directory is empty; nothing to push" {
		t.Fatalf("got error %q, want %q", err.Error(), "directory is empty; nothing to push")
	}
}

func Test_runPushRecursive_walkError(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test with non-existent directory
	opts := &pushOptions{
		Recursive: option.Recursive{
			Recursive:           true,
			MaxBlobsPerManifest: 1000,
		},
	}
	opts.FileRefs = []string{"/nonexistent/path/that/does/not/exist"}

	err := runPushRecursive(cmd, opts)
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func Test_pushArtifact_packError(t *testing.T) {
	packErr := errors.New("pack error")
	pack := func() (ocispec.Descriptor, error) {
		return ocispec.Descriptor{}, packErr
	}
	copyFn := func(_ ocispec.Descriptor) error {
		return nil
	}

	_, err := pushArtifact(nil, pack, copyFn)
	if err == nil {
		t.Fatal("expected error from pack")
	}
	if !errors.Is(err, packErr) {
		t.Fatalf("got error %v, want %v", err, packErr)
	}
}

func Test_pushArtifact_copyError(t *testing.T) {
	copyErr := errors.New("copy error")
	pack := func() (ocispec.Descriptor, error) {
		return ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			Size:      100,
		}, nil
	}
	copyFn := func(_ ocispec.Descriptor) error {
		return copyErr
	}

	_, err := pushArtifact(nil, pack, copyFn)
	if err == nil {
		t.Fatal("expected error from copy")
	}
	if !errors.Is(err, copyErr) {
		t.Fatalf("got error %v, want %v", err, copyErr)
	}
}

func Test_pushArtifact_success(t *testing.T) {
	expectedDesc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		Size:      100,
	}
	pack := func() (ocispec.Descriptor, error) {
		return expectedDesc, nil
	}
	copyFn := func(_ ocispec.Descriptor) error {
		return nil
	}

	desc, err := pushArtifact(nil, pack, copyFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Digest != expectedDesc.Digest {
		t.Fatalf("got digest %v, want %v", desc.Digest, expectedDesc.Digest)
	}
}

func Test_pushCmd(t *testing.T) {
	cmd := pushCmd()

	// Check command basics
	if cmd.Use == "" {
		t.Error("command Use is empty")
	}
	if cmd.Short == "" {
		t.Error("command Short is empty")
	}
	if cmd.Long == "" {
		t.Error("command Long is empty")
	}

	// Check flags are registered
	flags := []string{
		"config",
		"artifact-type",
		"concurrency",
		"recursive",
		"max-blobs-per-manifest",
		"preserve-empty-dirs",
		"follow-symlinks",
	}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag %q not registered", flag)
		}
	}
}

func Test_pushCmd_PreRunE_recursiveValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		setup   func(t *testing.T) string
		flags   map[string]string
		wantErr string
	}{
		{
			name: "recursive with config error",
			args: []string{"localhost:5000/test:v1", "somedir"},
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive": "true",
				"config":    "config.json",
			},
			wantErr: "--config cannot be used with --recursive",
		},
		{
			name: "recursive with multiple args error",
			args: []string{"localhost:5000/test:v1", "dir1", "dir2"},
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive": "true",
			},
			wantErr: "--recursive requires exactly one directory argument",
		},
		{
			name: "recursive with file instead of dir",
			args: []string{"localhost:5000/test:v1"},
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "file.txt")
				_ = os.WriteFile(file, []byte("test"), 0644)
				return file
			},
			flags: map[string]string{
				"recursive":               "true",
				"disable-path-validation": "true",
			},
			wantErr: "is not a directory; --recursive requires a directory",
		},
		{
			name: "recursive with absolute directory path",
			args: []string{"localhost:5000/test:v1"},
			setup: func(t *testing.T) string {
				t.Helper()
				// t.TempDir() returns an absolute path; recursive push must
				// disable the absolute-path validation that applies to
				// file-as-artifact arguments.
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive": "true",
			},
			wantErr: "",
		},
		{
			name: "recursive with nonexistent path",
			args: []string{"localhost:5000/test:v1", "nonexistent_path"},
			setup: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			flags: map[string]string{
				"recursive": "true",
			},
			wantErr: "cannot access",
		},
		{
			name: "recursive with artifact platform",
			args: []string{"localhost:5000/test:v1"},
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive":         "true",
				"artifact-platform": "linux/arm64",
			},
			wantErr: "--artifact-platform cannot be used with --recursive",
		},
		{
			name: "recursive with image-spec v1.0",
			args: []string{"localhost:5000/test:v1"},
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive":  "true",
				"image-spec": "v1.0",
			},
			wantErr: "--image-spec v1.0 cannot be used with --recursive",
		},
		{
			name: "max-blobs-per-manifest without recursive",
			args: []string{"localhost:5000/test:v1", "hi.txt"},
			setup: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			flags: map[string]string{
				"max-blobs-per-manifest":  "50",
				"disable-path-validation": "true",
			},
			wantErr: "--max-blobs-per-manifest requires --recursive",
		},
		{
			name: "recursive with invalid artifact type",
			args: []string{"localhost:5000/test:v1"},
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			flags: map[string]string{
				"recursive":     "true",
				"artifact-type": "not a media type",
			},
			wantErr: "invalid media type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := pushCmd()
			cmd.SetContext(context.Background())

			path := tt.setup(t)

			// Build args
			args := tt.args
			if path != "" && len(args) == 1 {
				args = append(args, path)
			}

			// Set flags
			for k, v := range tt.flags {
				if err := cmd.Flags().Set(k, v); err != nil {
					t.Fatalf("failed to set flag %s: %v", k, err)
				}
			}

			// Run PreRunE
			err := cmd.PreRunE(cmd, args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// newRecursiveTestDir creates a nested directory tree for recursive push tests.
func newRecursiveTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"a.txt":            "a",
		"sub/b.txt":        "b",
		"sub/nested/c.txt": "c",
	}
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runPushCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := pushCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())
	return cmd.Execute()
}

func Test_runPushRecursive_ociLayout(t *testing.T) {
	srcDir := newRecursiveTestDir(t)
	layoutDir := filepath.Join(t.TempDir(), "layout")
	exportPath := filepath.Join(t.TempDir(), "manifest.json")
	annotationFile := filepath.Join(t.TempDir(), "annotations.json")
	if err := os.WriteFile(annotationFile, []byte(`{"$manifest":{"k":"v"},"a.txt":{"file":"a"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--max-blobs-per-manifest", "1",
		"--annotation-file", annotationFile,
		"--export-manifest", exportPath,
		"--disable-path-validation",
		layoutDir+":v1,v2",
		srcDir,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store, err := oci.New(layoutDir)
	if err != nil {
		t.Fatalf("failed to open pushed layout: %v", err)
	}
	for _, tag := range []string{"v1", "v2"} {
		if _, err := store.Resolve(context.Background(), tag); err != nil {
			t.Errorf("expected tag %q to be resolvable, got error: %v", tag, err)
		}
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Errorf("expected exported manifest at %q: %v", exportPath, err)
	}
}

func Test_runPushRecursive_noTag(t *testing.T) {
	srcDir := newRecursiveTestDir(t)
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--format", "json",
		"--disable-path-validation",
		layoutDir,
		srcDir,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Test_runPushRecursive_jsonOutput verifies that --format json emits a single
// valid JSON document: the plain-text summary must not be appended to it.
func Test_runPushRecursive_jsonOutput(t *testing.T) {
	srcDir := newRecursiveTestDir(t)
	layoutDir := filepath.Join(t.TempDir(), "layout")

	cmd := pushCmd()
	var out strings.Builder
	cmd.SetArgs([]string{
		"--oci-layout",
		"--recursive",
		"--format", "json",
		"--disable-path-validation",
		layoutDir + ":v1",
		srcDir,
	})
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("output is not a single JSON document: %v\noutput:\n%s", err, out.String())
	}
}

func Test_runPushRecursive_preserveEmptyDirs(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(srcDir, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--preserve-empty-dirs",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func runPullCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := pullCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())
	return cmd.Execute()
}

// listTree returns the relative slash paths of all entries under root,
// suffixing directories with "/".
func listTree(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			rel += "/"
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func assertTree(t *testing.T, root string, want ...string) {
	t.Helper()
	got := listTree(t, root)
	if len(got) != len(want) {
		t.Fatalf("output tree = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("output tree = %v, want %v", got, want)
		}
	}
}

// Test_runPushRecursive_flatDirRoundTrip is a regression test: the root
// manifest descriptor used to carry AnnotationTitle = <directory basename>, so
// pulling a flat directory produced an extra file named after the source
// directory containing manifest JSON, and failed with "duplicate name" or
// "is a directory" when that name collided.
func Test_runPushRecursive_flatDirRoundTrip(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	// "src" matches the source directory basename: the historical collision case
	for _, name := range []string{"a", "src"} {
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	outDir := t.TempDir()
	if err := runPullCmd(t, "--oci-layout", layoutDir+":v1", "-o", outDir); err != nil {
		t.Fatalf("pull: %v", err)
	}
	assertTree(t, outDir, "a", "src")
	for _, name := range []string{"a", "src"} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != name {
			t.Errorf("file %q content = %q, want %q", name, data, name)
		}
	}
}

// Test_runPushRecursive_emptyDirRoundTrip verifies that --preserve-empty-dirs
// round-trips: pull restores the empty directory.
func Test_runPushRecursive_emptyDirRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "sub", "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--preserve-empty-dirs",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	outDir := t.TempDir()
	if err := runPullCmd(t, "--oci-layout", layoutDir+":v1", "-o", outDir); err != nil {
		t.Fatalf("pull: %v", err)
	}
	assertTree(t, outDir, "a.txt", "sub/", "sub/empty/")
}

// Test_runPushRecursive_annotationRename verifies that renaming a file through
// an annotation file works: the store must register the file under the
// annotated title, or copying fails with "not found".
func Test_runPushRecursive_annotationRename(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	annotationFile := filepath.Join(t.TempDir(), "annotations.json")
	if err := os.WriteFile(annotationFile, []byte(`{"a":{"org.opencontainers.image.title":"renamed"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--annotation-file", annotationFile,
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	outDir := t.TempDir()
	if err := runPullCmd(t, "--oci-layout", layoutDir+":v1", "-o", outDir); err != nil {
		t.Fatalf("pull: %v", err)
	}
	assertTree(t, outDir, "renamed")
}

// Test_runPushRecursive_symlinkNotFollowed verifies that without
// --follow-symlinks a symlink is skipped entirely: its target contents,
// which may live outside the pushed directory, must never be uploaded.
func Test_runPushRecursive_symlinkNotFollowed(t *testing.T) {
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret")
	if err := os.WriteFile(outsideFile, []byte("OUTSIDE DATA"), 0600); err != nil {
		t.Fatal(err)
	}

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(srcDir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	outDir := t.TempDir()
	if err := runPullCmd(t, "--oci-layout", layoutDir+":v1", "-o", outDir); err != nil {
		t.Fatalf("pull: %v", err)
	}
	assertTree(t, outDir, "a")
}

func Test_runPushRecursive_brokenSymlink(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.Symlink(filepath.Join(srcDir, "missing"), filepath.Join(srcDir, "broken")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--follow-symlinks",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	)
	if err == nil || !strings.Contains(err.Error(), "failed to walk directory") {
		t.Fatalf("got error %v, want walk error", err)
	}
}

// Test_runPushRecursive_manifestTitleAnnotation verifies that a user-supplied
// org.opencontainers.image.title manifest annotation does not end up on
// structural descriptors. The chunk, index, and root descriptors would all
// share that title, so pull failed with "duplicate name" and materialized
// manifest JSON as a file.
func Test_runPushRecursive_manifestTitleAnnotation(t *testing.T) {
	srcDir := t.TempDir()
	// files plus a subdirectory force a chunk manifest under an index
	if err := os.WriteFile(filepath.Join(srcDir, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b"), []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--annotation", "org.opencontainers.image.title=custom",
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	outDir := t.TempDir()
	if err := runPullCmd(t, "--oci-layout", layoutDir+":v1", "-o", outDir); err != nil {
		t.Fatalf("pull: %v", err)
	}
	assertTree(t, outDir, "a", "sub/", "sub/b")
}

// Test_runPushRecursive_configAnnotations verifies that $config annotations
// from an annotation file are applied to the config descriptors of recursive
// manifests. They used to be silently dropped: ordinary push passes them to
// PackManifestOptions, but the recursive builder pushed bare "{}" configs.
func Test_runPushRecursive_configAnnotations(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	annotationFile := filepath.Join(t.TempDir(), "annotations.json")
	if err := os.WriteFile(annotationFile, []byte(`{"$config":{"k":"v"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	layoutDir := filepath.Join(t.TempDir(), "layout")

	if err := runPushCmd(t,
		"--oci-layout",
		"--recursive",
		"--annotation-file", annotationFile,
		"--disable-path-validation",
		layoutDir+":v1",
		srcDir,
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	store, err := oci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rootDesc, err := store.Resolve(ctx, "v1")
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Fetch(ctx, rootDesc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	var manifest ocispec.Manifest
	if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Config.Annotations["k"] != "v" {
		t.Errorf("config annotations = %v, want k=v", manifest.Config.Annotations)
	}
}
