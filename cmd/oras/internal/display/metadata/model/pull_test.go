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

package model

import (
	"encoding/json"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content/file"
)

const pullTestDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var pullTestDescriptor = ocispec.Descriptor{
	MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
	Digest:    pullTestDigest,
	Size:      42,
}

func TestNewPull_files(t *testing.T) {
	pulledFile := File{
		Path: "/out/file.txt",
		Descriptor: FromDescriptor("localhost:5000/test", ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Digest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Size:      42,
		}),
	}
	tests := []struct {
		name  string
		files []File
		want  string
	}{
		{"nil files", nil, "[]"},
		{"empty files", []File{}, "[]"},
		{"one file", []File{pulledFile}, `[{"path":"/out/file.txt"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := json.Marshal(NewPull("localhost:5000/test@sha256:abc", tt.files))
			if err != nil {
				t.Fatalf("json.Marshal(NewPull()) error = %v", err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(content, &got); err != nil {
				t.Fatalf("NewPull() marshalled to invalid JSON %s: %v", content, err)
			}
			raw := string(got["files"])
			if tt.want == "[]" {
				if raw != "[]" {
					t.Errorf("NewPull() files = %s, want []", raw)
				}
			} else if len(raw) < len(tt.want) || raw[:len(tt.want)] != tt.want {
				t.Errorf("NewPull() files = %s, want it to start with %s", raw, tt.want)
			}
		})
	}
}

// A relative name is resolved against the output directory, while an
// absolute one is kept as given, so the recorded path is always absolute.
func TestNewFile_path(t *testing.T) {
	outputDir := t.TempDir()
	absolute := filepath.Join(outputDir, "elsewhere", "abs.txt")

	tests := []struct {
		name     string
		fileName string
		want     string
	}{
		{"relative name", "file.txt", filepath.Join(outputDir, "file.txt")},
		{"relative nested name", filepath.Join("sub", "file.txt"),
			filepath.Join(outputDir, "sub", "file.txt")},
		{"absolute name ignores the output dir", absolute, absolute},
		{"absolute name is cleaned",
			filepath.Join(outputDir, "a") + string(filepath.Separator) +
				".." + string(filepath.Separator) + "b.txt",
			filepath.Join(outputDir, "b.txt")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newFile(tt.fileName, outputDir, pullTestDescriptor,
				"layer/file.txt")
			if err != nil {
				t.Fatalf("newFile() error = %v, want nil", err)
			}
			if got.Path != tt.want {
				t.Errorf("newFile() path = %q, want %q", got.Path, tt.want)
			}
			if !filepath.IsAbs(got.Path) {
				t.Errorf("newFile() path = %q, want an absolute path", got.Path)
			}
		})
	}
}

// A descriptor marked for unpacking names a directory, so its path gets a
// trailing separator to tell it apart from a file of the same name.
func TestNewFile_unpackAnnotation(t *testing.T) {
	outputDir := t.TempDir()
	tests := []struct {
		name          string
		annotations   map[string]string
		wantSeparator bool
	}{
		{"no annotations", nil, false},
		{"unpack true", map[string]string{file.AnnotationUnpack: "true"}, true},
		{"unpack false", map[string]string{file.AnnotationUnpack: "false"}, false},
		{"unrelated annotation",
			map[string]string{ocispec.AnnotationTitle: "file.txt"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := pullTestDescriptor
			desc.Annotations = tt.annotations
			got, err := newFile("dir", outputDir, desc, "layer/dir")
			if err != nil {
				t.Fatalf("newFile() error = %v, want nil", err)
			}
			want := filepath.Join(outputDir, "dir")
			if tt.wantSeparator {
				want += string(filepath.Separator)
			}
			if got.Path != want {
				t.Errorf("newFile() path = %q, want %q", got.Path, want)
			}
		})
	}
}

// The descriptor half of a File is built from the descriptor path, not from
// the name the file was written under.
func TestNewFile_descriptor(t *testing.T) {
	got, err := newFile("file.txt", t.TempDir(), pullTestDescriptor,
		"localhost:5000/test")
	if err != nil {
		t.Fatalf("newFile() error = %v, want nil", err)
	}
	if want := "localhost:5000/test@" + pullTestDigest; got.Reference != want {
		t.Errorf("newFile() reference = %q, want %q", got.Reference, want)
	}
	if got.MediaType != pullTestDescriptor.MediaType {
		t.Errorf("newFile() mediaType = %q, want %q",
			got.MediaType, pullTestDescriptor.MediaType)
	}
	if got.Size != pullTestDescriptor.Size {
		t.Errorf("newFile() size = %d, want %d", got.Size, pullTestDescriptor.Size)
	}
}

func TestPulled_Add(t *testing.T) {
	outputDir := t.TempDir()
	var pulled Pulled

	if got := pulled.Files(); len(got) != 0 {
		t.Errorf("Files() = %v, want empty before anything is added", got)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		if err := pulled.Add(name, outputDir, pullTestDescriptor,
			"layer/"+name); err != nil {
			t.Fatalf("Add(%q) error = %v, want nil", name, err)
		}
	}

	got := pulled.Files()
	if len(got) != 2 {
		t.Fatalf("Files() = %v, want 2 entries", got)
	}
	// insertion order is preserved
	if want := filepath.Join(outputDir, "first.txt"); got[0].Path != want {
		t.Errorf("Files()[0] path = %q, want %q", got[0].Path, want)
	}
	if want := filepath.Join(outputDir, "second.txt"); got[1].Path != want {
		t.Errorf("Files()[1] path = %q, want %q", got[1].Path, want)
	}
}

// Files returns a copy, so a caller cannot reach back into the recorded
// files by writing to the slice it was handed.
func TestPulled_Files_returnsCopy(t *testing.T) {
	outputDir := t.TempDir()
	var pulled Pulled
	if err := pulled.Add("file.txt", outputDir, pullTestDescriptor,
		"layer/file.txt"); err != nil {
		t.Fatalf("Add() error = %v, want nil", err)
	}

	first := pulled.Files()
	first[0].Path = "tampered"

	second := pulled.Files()
	if second[0].Path == "tampered" {
		t.Error("Files() returned the stored slice; writing to it changed the recorded file")
	}
	if want := filepath.Join(outputDir, "file.txt"); second[0].Path != want {
		t.Errorf("Files() path = %q, want %q", second[0].Path, want)
	}
}

// Add and Files are mutex-protected, so exercise them together to give -race
// something to inspect, as TestTagged_Tags_concurrent does for the tag model.
func TestPulled_concurrent(t *testing.T) {
	outputDir := t.TempDir()
	var pulled Pulled
	names := []string{"a.txt", "b.txt", "c.txt", "d.txt",
		"e.txt", "f.txt", "g.txt", "h.txt"}
	done := make(chan error, len(names))
	for _, name := range names {
		go func() {
			err := pulled.Add(name, outputDir, pullTestDescriptor, "layer/"+name)
			_ = pulled.Files()
			done <- err
		}()
	}
	for range names {
		if err := <-done; err != nil {
			t.Errorf("Add() error = %v, want nil", err)
		}
	}
	if got := pulled.Files(); len(got) != len(names) {
		t.Errorf("Files() = %d entries, want %d", len(got), len(names))
	}
}
