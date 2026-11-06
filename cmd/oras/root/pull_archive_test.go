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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/file"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/option"
)

func archiveBlob(t *testing.T, src *memory.Store, name, mediaType string, data []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(mediaType, data)
	if name != "" {
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: name}
	}
	if err := src.Push(t.Context(), desc, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	return desc
}

func archiveManifest(t *testing.T, src *memory.Store, layers []ocispec.Descriptor, subject *ocispec.Descriptor, config ocispec.Descriptor) ocispec.Descriptor {
	t.Helper()
	data, err := json.Marshal(ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Layers: layers, Subject: subject, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	return archiveBlob(t, src, "", ocispec.MediaTypeImageManifest, data)
}

func archiveOptions(t *testing.T, src *memory.Store, root ocispec.Descriptor) *pullOptions {
	t.Helper()
	if err := src.Tag(t.Context(), root, "test"); err != nil {
		t.Fatal(err)
	}
	return &pullOptions{Target: option.Target{Reference: "test"}, OutputArchive: filepath.Join(t.TempDir(), "result.zip")}
}

func archiveContents(t *testing.T, filename string) (map[string]string, []string) {
	t.Helper()
	r, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	files := make(map[string]string)
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err := errors.Join(err, closeErr); err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(data)
	}
	return files, names
}

func TestPullArchive_ReproducibleAliases(t *testing.T) {
	src := memory.New()
	b := archiveBlob(t, src, "b.txt", "text/plain", []byte("shared payload"))
	a := b
	a.Annotations = map[string]string{ocispec.AnnotationTitle: "a.txt"}
	unnamed := archiveBlob(t, src, "", "text/plain", []byte("skip"))
	root := archiveManifest(t, src, []ocispec.Descriptor{b, unnamed, a}, nil, ocispec.DescriptorEmptyJSON)
	opts := archiveOptions(t, src, root)
	var first []byte
	for i := range 2 {
		if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(opts.OutputArchive)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = data
		} else if !bytes.Equal(first, data) {
			t.Fatal("identical input produced different ZIP bytes")
		}
	}
	got, names := archiveContents(t, opts.OutputArchive)
	if want := map[string]string{"a.txt": "shared payload", "b.txt": "shared payload"}; !maps.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(names, []string{"a.txt", "b.txt"}) {
		t.Fatalf("entry order = %v", names)
	}
	entries, err := os.ReadDir(filepath.Dir(opts.OutputArchive))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected staged or extracted files: %v, %v", entries, err)
	}
}

func TestPullArchive_SubjectAndConfig(t *testing.T) {
	src := memory.New()
	layer := archiveBlob(t, src, "subject.txt", "text/plain", []byte("subject"))
	subject := archiveManifest(t, src, []ocispec.Descriptor{layer}, nil, ocispec.DescriptorEmptyJSON)
	config := archiveBlob(t, src, "", "application/json", []byte(`{"answer":42}`))
	root := archiveManifest(t, src, nil, &subject, config)
	for _, include := range []bool{false, true} {
		opts := archiveOptions(t, src, root)
		opts.IncludeSubject = include
		opts.ManifestConfigRef = "config.json:application/json"
		if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err != nil {
			t.Fatal(err)
		}
		got, _ := archiveContents(t, opts.OutputArchive)
		if got["config.json"] != `{"answer":42}` {
			t.Fatalf("config missing: %v", got)
		}
		if _, exists := got["subject.txt"]; exists != include {
			t.Fatalf("subject exists = %v, include = %v", exists, include)
		}
	}
}

func TestPullArchive_Platform(t *testing.T) {
	src := memory.New()
	var manifests []ocispec.Descriptor
	for _, arch := range []string{"amd64", "arm64"} {
		config := archiveBlob(t, src, "", ocispec.MediaTypeImageConfig, []byte(`{"architecture":"`+arch+`","os":"linux"}`))
		layer := archiveBlob(t, src, "arch.txt", "text/plain", []byte(arch))
		desc := archiveManifest(t, src, []ocispec.Descriptor{layer}, nil, config)
		desc.Platform = &ocispec.Platform{OS: "linux", Architecture: arch}
		manifests = append(manifests, desc)
	}
	data, err := json.Marshal(ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: manifests})
	if err != nil {
		t.Fatal(err)
	}
	root := archiveBlob(t, src, "", ocispec.MediaTypeImageIndex, data)
	opts := archiveOptions(t, src, root)
	opts.Platform.Platform = &ocispec.Platform{OS: "linux", Architecture: "arm64"}
	if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err != nil {
		t.Fatal(err)
	}
	got, _ := archiveContents(t, opts.OutputArchive)
	if got["arch.txt"] != "arm64" {
		t.Fatalf("selected files = %v", got)
	}
}

func TestPullArchive_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := file.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	desc, err := store.Add(t.Context(), "src", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := store.Fetch(t.Context(), desc)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	src := memory.New()
	if err := src.Push(t.Context(), desc, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	root := archiveManifest(t, src, []ocispec.Descriptor{desc}, nil, ocispec.DescriptorEmptyJSON)
	opts := archiveOptions(t, src, root)
	if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err != nil {
		t.Fatal(err)
	}
	got, _ := archiveContents(t, opts.OutputArchive)
	if got["src/run"] != "#!/bin/sh\necho hello\n" {
		t.Fatalf("directory contents = %v", got)
	}
	if _, ok := got["src/empty/"]; !ok {
		t.Fatal("empty directory not preserved")
	}
}

func TestCollectPullArchive_MetadataOnly(t *testing.T) {
	src := memory.New()
	layer := archiveBlob(t, src, "payload", "application/octet-stream", []byte("large payload"))
	root := archiveManifest(t, src, []ocispec.Descriptor{layer}, nil, ocispec.DescriptorEmptyJSON)
	fetcher := content.FetcherFunc(func(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
		if desc.Digest == layer.Digest {
			t.Fatal("planning fetched a payload")
		}
		return src.Fetch(ctx, desc)
	})
	files, err := collectPullArchive(t.Context(), fetcher, root, &pullOptions{}, pullMetadataHandler{})
	if err != nil || len(files) != 1 {
		t.Fatalf("plan = %v, %v", files, err)
	}
}

func TestPullArchive_InvalidEntries(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", `C:\escape`, `dir\..\escape`, "a/../../b", ".", "a//b"} {
		t.Run(name, func(t *testing.T) {
			src := memory.New()
			layer := archiveBlob(t, src, name, "text/plain", []byte("data"))
			root := archiveManifest(t, src, []ocispec.Descriptor{layer}, nil, ocispec.DescriptorEmptyJSON)
			opts := archiveOptions(t, src, root)
			if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err == nil {
				t.Fatal("unsafe path accepted")
			}
			if _, err := os.Stat(opts.OutputArchive); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output created for unsafe input: %v", err)
			}
		})
	}
	for _, names := range [][]string{{"same", "same"}, {"a", "a/b"}} {
		src := memory.New()
		var layers []ocispec.Descriptor
		for i, name := range names {
			layers = append(layers, archiveBlob(t, src, name, "text/plain", []byte{byte(i)}))
		}
		root := archiveManifest(t, src, layers, nil, ocispec.DescriptorEmptyJSON)
		opts := archiveOptions(t, src, root)
		if err := runPullArchive(t.Context(), src, opts, pullMetadataHandler{}, status.DiscardHandler{}); err == nil {
			t.Fatalf("conflicting names accepted: %v", names)
		}
	}
}

func TestPullZIP_InvalidDirectories(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
	}{{"../escape", tar.TypeReg}, {"other/file", tar.TypeReg}, {"src/../../escape", tar.TypeReg}, {"src/link", tar.TypeSymlink}} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: tc.name, Typeflag: tc.kind, Linkname: "../../escape", Mode: 0644}); err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(tw.Close(), gz.Close()); err != nil {
				t.Fatal(err)
			}
			src := memory.New()
			desc := archiveBlob(t, src, "src", "application/gzip", buf.Bytes())
			desc.Annotations[file.AnnotationUnpack] = "true"
			var output bytes.Buffer
			if err := writePullZIP(t.Context(), &output, src, []ocispec.Descriptor{desc}, status.DiscardHandler{}); err == nil {
				t.Fatal("invalid directory accepted")
			}
		})
	}
}

func TestSavePullArchive_FailureAndKeepOld(t *testing.T) {
	failed := errors.New("transfer failed")
	for _, exists := range []bool{false, true} {
		for _, keep := range []bool{false, true} {
			name := filepath.Join(t.TempDir(), "result.zip")
			if exists {
				if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := savePullArchive(name, keep, func(w io.Writer) error {
				_, _ = io.WriteString(w, "partial")
				return failed
			})
			if exists && keep {
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("keep old error = %v", err)
				}
			} else if !errors.Is(err, failed) {
				t.Fatalf("write error = %v", err)
			}
			data, err := os.ReadFile(name)
			if exists && (err != nil || string(data) != "original") {
				t.Fatalf("original replaced on failure: %q, %v", data, err)
			}
			if !exists && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial output left: %q, %v", data, err)
			}
			staged, err := filepath.Glob(filepath.Join(filepath.Dir(name), ".oras-archive-*"))
			if err != nil || len(staged) != 0 {
				t.Fatalf("staged output left: %v, %v", staged, err)
			}
		}
	}
}

func TestPullZIP_CorruptContent(t *testing.T) {
	desc := content.NewDescriptorFromBytes("text/plain", []byte("expected"))
	desc.Annotations = map[string]string{ocispec.AnnotationTitle: "data"}
	fetcher := content.FetcherFunc(func(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("corrupt!")), nil
	})
	var output bytes.Buffer
	err := writePullZIP(t.Context(), &output, fetcher, []ocispec.Descriptor{desc}, status.DiscardHandler{})
	if !errors.Is(err, content.ErrMismatchedDigest) {
		t.Fatalf("digest error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := writePullZIP(ctx, &output, fetcher, []ocispec.Descriptor{desc}, status.DiscardHandler{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestPullArchive_FlagConflicts(t *testing.T) {
	for _, flag := range []string{"output", "allow-path-traversal", "format"} {
		cmd := pullCmd()
		if err := cmd.Flags().Set("output-archive", "result.zip"); err != nil {
			t.Fatal(err)
		}
		value := "true"
		switch flag {
		case "output":
			value = "out"
		case "format":
			value = "json"
		}
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
		if err := cmd.ValidateFlagGroups(); err == nil {
			t.Fatalf("--output-archive accepted with --%s", flag)
		}
	}
}
