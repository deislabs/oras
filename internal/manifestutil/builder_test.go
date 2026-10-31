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

package manifestutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras/internal/dir"
)

// TestBuildFromNode_noAnnotationTitleOnIndexDescriptors is a regression test for
// the bug reported in https://github.com/oras-project/oras/pull/1951 where
// oras pull would fail with "open <path>: is a directory" after a recursive push.
//
// Root cause: descriptors stored in an index's manifests list had
// org.opencontainers.image.title = "<dirname>". oras pull's PostCopy calls
// content.Successors on each copied descriptor; for any successor that has
// AnnotationTitle it calls metadataHandler.OnFilePulled, which tries to
// open/create that path as a file, failing with "is a directory" when a
// directory with that name already exists on disk.
//
// Two shapes trigger the bug:
//  1. Root has only subdirectories (child Image Index descriptors carry the title)
//  2. Root has both files and subdirectories (chunk manifest descriptors stored
//     inside the index carry the directory name as AnnotationTitle)
func TestBuildFromNode_noAnnotationTitleOnIndexDescriptors(t *testing.T) {
	t.Run("only subdirs at root", func(t *testing.T) {
		ctx := context.Background()
		store := memory.New()
		b := NewBuilder(store, BuilderOptions{MaxBlobsPerManifest: 1000})

		root := &dir.Node{
			Name:  "root",
			Path:  ".",
			IsDir: true,
			Children: []*dir.Node{
				{
					Name:  "subdir",
					Path:  "subdir",
					IsDir: true,
					Children: []*dir.Node{
						{Name: "file.txt", Path: "subdir/file.txt", IsDir: false},
					},
				},
			},
		}
		fileDesc := content.NewDescriptorFromBytes("application/vnd.oci.image.layer.v1.tar", []byte("hello"))
		fileDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "subdir/file.txt"}
		fileDescs := map[string]ocispec.Descriptor{"subdir/file.txt": fileDesc}

		assertNoAnnotationTitleInAllIndexes(ctx, t, store, b, root, fileDescs)
	})

	t.Run("files and subdirs at root", func(t *testing.T) {
		ctx := context.Background()
		store := memory.New()
		b := NewBuilder(store, BuilderOptions{MaxBlobsPerManifest: 1000})

		// This shape triggers the second variant of the bug: the root has both
		// files and a subdirectory, so buildNode creates a chunk manifest for
		// the root files. If that chunk manifest descriptor has AnnotationTitle
		// = root.Name, oras pull calls OnFilePulled(root.Name) which fails when
		// root.Name already exists as a directory.
		//
		// Similarly, "subdir" has files AND a nested subdirectory, so it also
		// produces a chunk manifest with AnnotationTitle = "subdir", which
		// fails when the subdir directory exists on disk.
		root := &dir.Node{
			Name:  "mydir",
			Path:  ".",
			IsDir: true,
			Children: []*dir.Node{
				{Name: "root.txt", Path: "root.txt", IsDir: false},
				{
					Name:  "subdir",
					Path:  "subdir",
					IsDir: true,
					Children: []*dir.Node{
						{Name: "sub.txt", Path: "subdir/sub.txt", IsDir: false},
						{
							Name:  "nested",
							Path:  "subdir/nested",
							IsDir: true,
							Children: []*dir.Node{
								{Name: "deep.txt", Path: "subdir/nested/deep.txt", IsDir: false},
							},
						},
					},
				},
			},
		}
		mkDesc := func(path, data string) ocispec.Descriptor {
			d := content.NewDescriptorFromBytes("application/octet-stream", []byte(data))
			d.Annotations = map[string]string{ocispec.AnnotationTitle: path}
			return d
		}
		fileDescs := map[string]ocispec.Descriptor{
			"root.txt":               mkDesc("root.txt", "root"),
			"subdir/sub.txt":         mkDesc("subdir/sub.txt", "sub"),
			"subdir/nested/deep.txt": mkDesc("subdir/nested/deep.txt", "deep"),
		}

		assertNoAnnotationTitleInAllIndexes(ctx, t, store, b, root, fileDescs)
	})
}

// assertNoAnnotationTitleInAllIndexes builds from node and recursively checks
// every Image Index in the store for manifest entries that have AnnotationTitle.
func assertNoAnnotationTitleInAllIndexes(ctx context.Context, t *testing.T, store interface {
	content.Pusher
	content.Fetcher
}, b *Builder, root *dir.Node, fileDescs map[string]ocispec.Descriptor) {
	t.Helper()

	res, err := b.BuildFromNode(ctx, root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if res.Root.MediaType != ocispec.MediaTypeImageIndex {
		t.Fatalf("root MediaType = %q, want %q", res.Root.MediaType, ocispec.MediaTypeImageIndex)
	}

	// Recursively walk every index and assert no manifest entry has AnnotationTitle.
	var checkIndex func(desc ocispec.Descriptor, label string)
	checkIndex = func(desc ocispec.Descriptor, label string) {
		rc, err := store.Fetch(ctx, desc)
		if err != nil {
			t.Fatalf("Fetch %s: %v", label, err)
		}
		defer rc.Close()

		var idx ocispec.Index
		if err := json.NewDecoder(rc).Decode(&idx); err != nil {
			t.Fatalf("decode %s: %v", label, err)
		}

		for i, m := range idx.Manifests {
			if title, ok := m.Annotations[ocispec.AnnotationTitle]; ok {
				t.Errorf("%s manifest[%d] has AnnotationTitle=%q: "+
					"oras pull will call OnFilePulled(%q) and fail with \"is a directory\"",
					label, i, title, title)
			}
			// Recurse into child indexes.
			if m.MediaType == ocispec.MediaTypeImageIndex {
				checkIndex(m, label+"/manifest["+string(rune('0'+i))+"]")
			}
		}
	}
	checkIndex(res.Root, "root")
}

// TestBuildFromNode_noAnnotationTitleOnRoot is a regression test for the flat
// root directory case: the root manifest descriptor carried AnnotationTitle =
// <directory basename>, which OCI layouts and registries preserve, so oras
// pull materialized the root manifest JSON as an extra file named after the
// pushed directory, or failed with "duplicate name" / "is a directory" when
// that name collided with real content.
func TestBuildFromNode_noAnnotationTitleOnRoot(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	b := NewBuilder(store, BuilderOptions{MaxBlobsPerManifest: 1000})

	// Flat tree: only files at the root, so the root is a plain manifest.
	root := &dir.Node{
		Name:  "src",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a", Path: "a", IsDir: false},
		},
	}
	fileDesc := content.NewDescriptorFromBytes("application/octet-stream", []byte("a"))
	fileDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "a"}
	fileDescs := map[string]ocispec.Descriptor{"a": fileDesc}

	res, err := b.BuildFromNode(ctx, root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if res.Root.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("root MediaType = %q, want %q", res.Root.MediaType, ocispec.MediaTypeImageManifest)
	}
	if title, ok := res.Root.Annotations[ocispec.AnnotationTitle]; ok {
		t.Errorf("root descriptor has AnnotationTitle=%q: oras pull will materialize the manifest as a file", title)
	}

	rc, err := store.Fetch(ctx, res.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var manifest ocispec.Manifest
	if err := json.NewDecoder(rc).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if title, ok := manifest.Annotations[ocispec.AnnotationTitle]; ok {
		t.Errorf("root manifest content has AnnotationTitle=%q", title)
	}
}

// TestBuildFromNode_manifestAnnotationsOnChunkManifests verifies that
// user-supplied manifest annotations are applied to chunk manifests too, as
// the ManifestAnnotations option documents ("annotations to add to all
// manifests").
func TestBuildFromNode_manifestAnnotationsOnChunkManifests(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	b := NewBuilder(store, BuilderOptions{
		MaxBlobsPerManifest: 1,
		ManifestAnnotations: map[string]string{"hello": "world"},
	})

	// Files plus a subdirectory force the root files into chunk manifests.
	root := &dir.Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a", Path: "a", IsDir: false},
			{Name: "b", Path: "b", IsDir: false},
			{
				Name:  "subdir",
				Path:  "subdir",
				IsDir: true,
				Children: []*dir.Node{
					{Name: "c", Path: "subdir/c", IsDir: false},
				},
			},
		},
	}
	mkDesc := func(path, data string) ocispec.Descriptor {
		d := content.NewDescriptorFromBytes("application/octet-stream", []byte(data))
		d.Annotations = map[string]string{ocispec.AnnotationTitle: path}
		return d
	}
	fileDescs := map[string]ocispec.Descriptor{
		"a":        mkDesc("a", "a"),
		"b":        mkDesc("b", "b"),
		"subdir/c": mkDesc("subdir/c", "c"),
	}

	res, err := b.BuildFromNode(ctx, root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	for i, desc := range res.AllDescriptors {
		rc, err := store.Fetch(ctx, desc)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Annotations map[string]string `json:"annotations"`
		}
		err = json.NewDecoder(rc).Decode(&m)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if m.Annotations["hello"] != "world" {
			t.Errorf("descriptor[%d] (%s) missing manifest annotation hello=world, got %v", i, desc.MediaType, m.Annotations)
		}
	}
}

// TestBuildFromNode_titleManifestAnnotationNotOnDescriptors verifies that a
// user-supplied org.opencontainers.image.title manifest annotation stays in
// the manifest content but never lands on structural descriptors, where it
// would make oras pull materialize manifests as files and fail with
// "duplicate name" when several structural descriptors share the title.
func TestBuildFromNode_titleManifestAnnotationNotOnDescriptors(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	b := NewBuilder(store, BuilderOptions{
		MaxBlobsPerManifest: 1000,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationTitle: "custom",
			"hello":                 "world",
		},
	})

	// files plus a subdirectory produce a chunk manifest under an index
	root := &dir.Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a", Path: "a", IsDir: false},
			{
				Name:  "sub",
				Path:  "sub",
				IsDir: true,
				Children: []*dir.Node{
					{Name: "b", Path: "sub/b", IsDir: false},
				},
			},
		},
	}
	mkDesc := func(path, data string) ocispec.Descriptor {
		d := content.NewDescriptorFromBytes("application/octet-stream", []byte(data))
		d.Annotations = map[string]string{ocispec.AnnotationTitle: path}
		return d
	}
	fileDescs := map[string]ocispec.Descriptor{
		"a":     mkDesc("a", "a"),
		"sub/b": mkDesc("sub/b", "b"),
	}

	res, err := b.BuildFromNode(ctx, root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if title, ok := res.Root.Annotations[ocispec.AnnotationTitle]; ok {
		t.Errorf("root descriptor has AnnotationTitle=%q", title)
	}
	if res.Root.Annotations["hello"] != "world" {
		t.Errorf("root descriptor lost non-title manifest annotation: %v", res.Root.Annotations)
	}
	for i, desc := range res.AllDescriptors {
		if descriptor := desc.Annotations; descriptor != nil {
			if title, ok := descriptor[ocispec.AnnotationTitle]; ok {
				t.Errorf("descriptor[%d] (%s) has AnnotationTitle=%q", i, desc.MediaType, title)
			}
		}
		// the manifest content must keep the user's annotations verbatim
		rc, err := store.Fetch(ctx, desc)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Annotations map[string]string `json:"annotations"`
		}
		err = json.NewDecoder(rc).Decode(&m)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if m.Annotations[ocispec.AnnotationTitle] != "custom" || m.Annotations["hello"] != "world" {
			t.Errorf("descriptor[%d] (%s) content annotations = %v, want title=custom and hello=world", i, desc.MediaType, m.Annotations)
		}
	}
}

// TestBuildFromNode_invalidArtifactType verifies that artifact types are held
// to the same media type validation that oras.PackManifest applies in the
// ordinary push path.
func TestBuildFromNode_invalidArtifactType(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	b := NewBuilder(store, BuilderOptions{ArtifactType: "not a media type"})

	root := &dir.Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a", Path: "a", IsDir: false},
		},
	}
	fileDesc := content.NewDescriptorFromBytes("application/octet-stream", []byte("a"))
	fileDescs := map[string]ocispec.Descriptor{"a": fileDesc}

	_, err := b.BuildFromNode(ctx, root, fileDescs)
	if !errors.Is(err, errdef.ErrInvalidMediaType) {
		t.Fatalf("got error %v, want %v", err, errdef.ErrInvalidMediaType)
	}
}

func TestNewBuilder(t *testing.T) {
	store := memory.New()

	t.Run("default max blobs", func(t *testing.T) {
		builder := NewBuilder(store, BuilderOptions{})
		if builder.opts.MaxBlobsPerManifest != 1000 {
			t.Errorf("MaxBlobsPerManifest = %d, want 1000", builder.opts.MaxBlobsPerManifest)
		}
	})

	t.Run("custom max blobs", func(t *testing.T) {
		builder := NewBuilder(store, BuilderOptions{MaxBlobsPerManifest: 500})
		if builder.opts.MaxBlobsPerManifest != 500 {
			t.Errorf("MaxBlobsPerManifest = %d, want 500", builder.opts.MaxBlobsPerManifest)
		}
	})
}

func TestBuilder_BuildFromNode_FilesOnly(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	builder := NewBuilder(store, BuilderOptions{
		MaxBlobsPerManifest: 1000,
	})

	// Create a simple directory structure with only files
	rootNode := &dir.Node{
		Name:  "testdir",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "file1.txt", Path: "file1.txt", IsDir: false, Size: 100},
			{Name: "file2.txt", Path: "file2.txt", IsDir: false, Size: 200},
		},
	}

	// Create mock file descriptors
	fileDescs := map[string]ocispec.Descriptor{
		"file1.txt": {
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			Size:      100,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: "file1.txt",
			},
		},
		"file2.txt": {
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			Size:      200,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: "file2.txt",
			},
		},
	}

	result, err := builder.BuildFromNode(ctx, rootNode, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}

	if result.Root.Digest == "" {
		t.Error("result.Root.Digest should not be empty")
	}

	if result.ManifestCount != 1 {
		t.Errorf("ManifestCount = %d, want 1", result.ManifestCount)
	}

	if result.IndexCount != 0 {
		t.Errorf("IndexCount = %d, want 0", result.IndexCount)
	}
}

func TestBuilder_BuildFromNode_WithSubdirs(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	builder := NewBuilder(store, BuilderOptions{
		MaxBlobsPerManifest: 1000,
	})

	// Create a directory structure with subdirectories
	rootNode := &dir.Node{
		Name:  "testdir",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "file1.txt", Path: "file1.txt", IsDir: false, Size: 100},
			{
				Name:  "subdir",
				Path:  "subdir",
				IsDir: true,
				Children: []*dir.Node{
					{Name: "file2.txt", Path: "subdir/file2.txt", IsDir: false, Size: 200},
				},
			},
		},
	}

	// Create mock file descriptors
	fileDescs := map[string]ocispec.Descriptor{
		"file1.txt": {
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			Size:      100,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: "file1.txt",
			},
		},
		"subdir/file2.txt": {
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			Size:      200,
			Annotations: map[string]string{
				ocispec.AnnotationTitle: "file2.txt",
			},
		},
	}

	result, err := builder.BuildFromNode(ctx, rootNode, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}

	if result.Root.Digest == "" {
		t.Error("result.Root.Digest should not be empty")
	}

	// Should have index at root (1) + manifest for root files (1) + manifest for subdir (1)
	if result.ManifestCount != 2 {
		t.Errorf("ManifestCount = %d, want 2", result.ManifestCount)
	}

	if result.IndexCount != 1 {
		t.Errorf("IndexCount = %d, want 1", result.IndexCount)
	}
}

func TestBuilder_BuildFromNode_EmptyDir(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	t.Run("without preserve empty", func(t *testing.T) {
		builder := NewBuilder(store, BuilderOptions{
			PreserveEmptyDirs: false,
		})

		rootNode := &dir.Node{
			Name:     "emptydir",
			Path:     ".",
			IsDir:    true,
			Children: []*dir.Node{},
		}

		result, err := builder.BuildFromNode(ctx, rootNode, map[string]ocispec.Descriptor{})
		if err != nil {
			t.Fatalf("BuildFromNode() error = %v", err)
		}

		if result.Root.Digest != "" {
			t.Error("empty dir without preserve should have empty digest")
		}
	})

	t.Run("with preserve empty", func(t *testing.T) {
		builder := NewBuilder(store, BuilderOptions{
			PreserveEmptyDirs: true,
		})

		rootNode := &dir.Node{
			Name:     "emptydir",
			Path:     ".",
			IsDir:    true,
			Children: []*dir.Node{},
		}

		result, err := builder.BuildFromNode(ctx, rootNode, map[string]ocispec.Descriptor{})
		if err != nil {
			t.Fatalf("BuildFromNode() error = %v", err)
		}

		if result.Root.Digest == "" {
			t.Error("empty dir with preserve should have non-empty digest")
		}
	})
}

func TestChunkDescriptors(t *testing.T) {
	descs := []ocispec.Descriptor{
		{Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
		{Digest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
		{Digest: "sha256:3333333333333333333333333333333333333333333333333333333333333333"},
		{Digest: "sha256:4444444444444444444444444444444444444444444444444444444444444444"},
		{Digest: "sha256:5555555555555555555555555555555555555555555555555555555555555555"},
	}

	tests := []struct {
		name          string
		maxSize       int
		expectedCount int
	}{
		{"no chunking", 10, 1},
		{"exact fit", 5, 1},
		{"two chunks", 3, 2},
		{"three chunks", 2, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chunkDescriptors(descs, tt.maxSize)
			if len(chunks) != tt.expectedCount {
				t.Errorf("chunkDescriptors() = %d chunks, want %d", len(chunks), tt.expectedCount)
			}
		})
	}
}

// failingPusher fails pushes whose media type matches failOn with err.
type failingPusher struct {
	failOn string
	err    error
}

func (p *failingPusher) Push(_ context.Context, desc ocispec.Descriptor, r io.Reader) error {
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	if desc.MediaType == p.failOn {
		return p.err
	}
	return nil
}

func testFileDesc(i int) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.layer.v1.tar",
		Digest:    content.NewDescriptorFromBytes("", []byte(strconv.Itoa(i))).Digest,
		Size:      1,
	}
}

// nestedTestNode returns a root dir containing a file and a subdir with a file.
func nestedTestNode() (*dir.Node, map[string]ocispec.Descriptor) {
	root := &dir.Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a.txt", Path: "a.txt"},
			{Name: "sub", Path: "sub", IsDir: true, Children: []*dir.Node{
				{Name: "b.txt", Path: "sub/b.txt"},
			}},
		},
	}
	return root, map[string]ocispec.Descriptor{
		"a.txt":     testFileDesc(1),
		"sub/b.txt": testFileDesc(2),
	}
}

func TestBuilder_BuildFromNode_PushErrors(t *testing.T) {
	pushErr := errors.New("push failed")
	for _, mediaType := range []string{
		ocispec.MediaTypeImageConfig,
		ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex,
	} {
		t.Run(mediaType, func(t *testing.T) {
			root, fileDescs := nestedTestNode()
			builder := NewBuilder(&failingPusher{failOn: mediaType, err: pushErr}, BuilderOptions{})
			if _, err := builder.BuildFromNode(context.Background(), root, fileDescs); !errors.Is(err, pushErr) {
				t.Fatalf("BuildFromNode() error = %v, want %v", err, pushErr)
			}
		})
	}
}

func TestBuilder_BuildFromNode_ChunkPushError(t *testing.T) {
	pushErr := errors.New("push failed")
	root := &dir.Node{Name: "root", Path: ".", IsDir: true}
	fileDescs := map[string]ocispec.Descriptor{}
	for i := range 3 {
		name := fmt.Sprintf("f%d", i)
		root.Children = append(root.Children, &dir.Node{Name: name, Path: name})
		fileDescs[name] = testFileDesc(i)
	}
	builder := NewBuilder(&failingPusher{failOn: ocispec.MediaTypeImageManifest, err: pushErr}, BuilderOptions{MaxBlobsPerManifest: 1})
	if _, err := builder.BuildFromNode(context.Background(), root, fileDescs); !errors.Is(err, pushErr) {
		t.Fatalf("BuildFromNode() error = %v, want %v", err, pushErr)
	}
}

func TestBuilder_BuildFromNode_AlreadyExists(t *testing.T) {
	for _, mediaType := range []string{
		ocispec.MediaTypeImageConfig,
		ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex,
	} {
		t.Run(mediaType, func(t *testing.T) {
			root, fileDescs := nestedTestNode()
			builder := NewBuilder(&failingPusher{failOn: mediaType, err: errdef.ErrAlreadyExists}, BuilderOptions{})
			if _, err := builder.BuildFromNode(context.Background(), root, fileDescs); err != nil {
				t.Fatalf("BuildFromNode() error = %v", err)
			}
		})
	}
}

func TestBuilder_BuildFromNode_ArtifactType(t *testing.T) {
	artifactType := "application/vnd.example"
	root, fileDescs := nestedTestNode()
	builder := NewBuilder(memory.New(), BuilderOptions{ArtifactType: artifactType})
	result, err := builder.BuildFromNode(context.Background(), root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if result.Root.ArtifactType != artifactType {
		t.Errorf("root ArtifactType = %q, want %q", result.Root.ArtifactType, artifactType)
	}
	if result.IndexCount != 1 || result.ManifestCount != 2 {
		t.Errorf("IndexCount = %d, ManifestCount = %d, want 1, 2", result.IndexCount, result.ManifestCount)
	}
}

func TestBuilder_BuildFromNode_FileNode(t *testing.T) {
	builder := NewBuilder(memory.New(), BuilderOptions{})
	want := testFileDesc(1)

	result, err := builder.BuildFromNode(context.Background(), &dir.Node{Name: "a", Path: "a"}, map[string]ocispec.Descriptor{"a": want})
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if result.Root.Digest != want.Digest {
		t.Errorf("Root = %v, want %v", result.Root.Digest, want.Digest)
	}

	result, err = builder.BuildFromNode(context.Background(), &dir.Node{Name: "missing", Path: "missing"}, nil)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if result.Root.Digest != "" {
		t.Errorf("Root = %v, want empty descriptor", result.Root.Digest)
	}
}

func TestBuilder_BuildFromNode_SkipsEmptySubdir(t *testing.T) {
	root := &dir.Node{
		Name:  "root",
		Path:  ".",
		IsDir: true,
		Children: []*dir.Node{
			{Name: "a.txt", Path: "a.txt"},
			{Name: "empty", Path: "empty", IsDir: true},
		},
	}
	builder := NewBuilder(memory.New(), BuilderOptions{})
	result, err := builder.BuildFromNode(context.Background(), root, map[string]ocispec.Descriptor{"a.txt": testFileDesc(1)})
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}
	if result.Root.MediaType != ocispec.MediaTypeImageManifest || result.IndexCount != 0 {
		t.Errorf("got root %s with %d indexes, want a single manifest", result.Root.MediaType, result.IndexCount)
	}
}

func TestBuilder_BuildFromNode_ChunkIndexAnnotation(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	root := &dir.Node{Name: "root", Path: ".", IsDir: true}
	fileDescs := map[string]ocispec.Descriptor{}
	for i := range 12 {
		name := fmt.Sprintf("f%02d", i)
		root.Children = append(root.Children, &dir.Node{Name: name, Path: name})
		fileDescs[name] = testFileDesc(i)
	}

	builder := NewBuilder(store, BuilderOptions{MaxBlobsPerManifest: 1})
	result, err := builder.BuildFromNode(ctx, root, fileDescs)
	if err != nil {
		t.Fatalf("BuildFromNode() error = %v", err)
	}

	indexBytes, err := content.FetchAll(ctx, store, result.Root)
	if err != nil {
		t.Fatal(err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Manifests) != 12 {
		t.Fatalf("got %d manifests, want 12", len(index.Manifests))
	}
	for i, m := range index.Manifests {
		if got, want := m.Annotations["org.oras.content.chunk.index"], strconv.Itoa(i); got != want {
			t.Errorf("manifest %d chunk index = %q, want %q", i, got, want)
		}
	}
}
