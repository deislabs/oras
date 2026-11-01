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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	metadatajson "oras.land/oras/cmd/oras/internal/display/metadata/json"
	"oras.land/oras/cmd/oras/internal/display/status"
	orerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
)

type pullMetadataHandler struct{}

func (pullMetadataHandler) Render() error { return nil }

func (pullMetadataHandler) OnLayerSkipped(ocispec.Descriptor) error { return nil }

func (pullMetadataHandler) OnFilePulled(string, string, ocispec.Descriptor, string) error {
	return nil
}

func (pullMetadataHandler) OnPulled(*option.Target, ocispec.Descriptor) {}

func Test_runPull_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test
	opts := &pullOptions{
		Format: option.Format{
			Type: "unknown",
		},
	}
	got := runPull(cmd, opts).Error()
	want := orerrors.UnsupportedFormatTypeError(opts.Format.Type).Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNewPullCleanup(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")

	cleanup, err := newPullCleanup(output)
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	wantRoot, err := filepath.Abs(output)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if cleanup.root != wantRoot {
		t.Fatalf("newPullCleanup() root = %q, want %q", cleanup.root, wantRoot)
	}
	if len(cleanup.seen) != 0 {
		t.Fatalf("newPullCleanup() seen = %d, want 0", len(cleanup.seen))
	}
	if len(cleanup.created) != 0 {
		t.Fatalf("newPullCleanup() created = %d, want 0", len(cleanup.created))
	}
}

func TestDoPull_StopsBeforeCopyWhenTrackingFails(t *testing.T) {
	wantedErr := errors.New("track target failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPullStatusHandler{Recorder: recorder, TrackErr: wantedErr}
	cleanup, err := newPullCleanup(t.TempDir())
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	_, err = doPull(
		context.Background(),
		memory.New(),
		memory.New(),
		oras.CopyOptions{},
		pullMetadataHandler{},
		statusHandler,
		&pullOptions{},
		cleanup,
	)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("doPull() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"TrackTarget"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status handler calls = %v, want %v", got, want)
	}
}

func TestDoPull_TracksFilePath(t *testing.T) {
	ctx := context.Background()

	src := memory.New()
	output := t.TempDir()

	dst, err := file.New(output)
	if err != nil {
		t.Fatalf("file.New() error = %v", err)
	}

	content := []byte("test content\n")
	filePath := "tool"

	layerDesc := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: filePath,
		},
	}

	if err := src.Push(ctx, layerDesc, bytes.NewReader(content)); err != nil {
		t.Fatalf("Push() layer error = %v", err)
	}

	skippedContent := []byte("unnamed layer\n")
	skippedLayer := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(skippedContent),
		Size:      int64(len(skippedContent)),
	}
	if err := src.Push(ctx, skippedLayer, bytes.NewReader(skippedContent)); err != nil {
		t.Fatalf("Push() skipped layer error = %v", err)
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    ocispec.DescriptorEmptyJSON.Digest,
			Size:      ocispec.DescriptorEmptyJSON.Size,
		},
		Layers: []ocispec.Descriptor{
			layerDesc,
			skippedLayer,
		},
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}

	if err := src.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		t.Fatalf("Push() manifest error = %v", err)
	}
	if err := src.Tag(ctx, manifestDesc, "test"); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}

	cleanup, err := newPullCleanup(output)
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPullStatusHandler{Recorder: recorder}
	metadataHandler := &pullMetadataHandler{}

	po := &pullOptions{
		Target: option.Target{
			Reference: "test",
		},
		Output: output,
	}

	_, err = doPull(
		ctx,
		src,
		dst,
		oras.CopyOptions{},
		metadataHandler,
		statusHandler,
		po,
		cleanup,
	)
	if err != nil {
		t.Fatalf("doPull() error = %v", err)
	}

	pulledPath := filepath.Join(output, filePath)
	data, err := os.ReadFile(pulledPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("pulled content = %q, want %q", data, content)
	}
	wantCalls := []string{"TrackTarget", "OnNodeDownloading", "OnNodeProcessing", "OnNodeSkipped", "OnNodeDownloading", "OnNodeDownloaded", "OnNodeDownloaded", "StopTrackTarget"}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("status handler calls = %v, want %v", got, wantCalls)
	}
}

func TestDoPull_OnNodeProcessingError(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	output := t.TempDir()
	dst, err := file.New(output)
	if err != nil {
		t.Fatalf("file.New() error = %v", err)
	}

	layer := []byte("test content\n")
	layerDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayer,
		Digest:    digest.FromBytes(layer),
		Size:      int64(len(layer)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "tool",
		},
	}
	if err := src.Push(ctx, layerDesc, bytes.NewReader(layer)); err != nil {
		t.Fatalf("Push() layer error = %v", err)
	}
	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.DescriptorEmptyJSON,
		Layers:    []ocispec.Descriptor{layerDesc},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
	if err := src.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		t.Fatalf("Push() manifest error = %v", err)
	}
	if err := src.Tag(ctx, manifestDesc, "test"); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}

	wantedErr := errors.New("processing failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPullStatusHandler{Recorder: recorder, NodeProcessingErr: wantedErr}
	cleanup, err := newPullCleanup(output)
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}
	po := &pullOptions{Target: option.Target{Reference: "test"}, Output: output}
	_, err = doPull(ctx, src, dst, oras.CopyOptions{}, pullMetadataHandler{}, statusHandler, po, cleanup)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("doPull() error = %v, want %v", err, wantedErr)
	}
	wantCalls := []string{"TrackTarget", "OnNodeDownloading", "OnNodeProcessing", "StopTrackTarget"}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("status handler calls = %v, want %v", got, wantCalls)
	}
	if _, err := os.Stat(filepath.Join(output, "tool")); !os.IsNotExist(err) {
		t.Fatalf("output file exists after processing failed")
	}
}

func TestDoPull_CleansUpDuplicateFile(t *testing.T) {
	ctx := context.Background()

	src := memory.New()
	output := t.TempDir()

	dst, err := file.New(output)
	if err != nil {
		t.Fatalf("file.New() error = %v", err)
	}

	content1 := []byte("first content\n")
	content2 := []byte("second content\n")
	filePath := "tool"

	layer1 := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content1),
		Size:      int64(len(content1)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: filePath,
		},
	}

	layer2 := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content2),
		Size:      int64(len(content2)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: filePath,
		},
	}

	if err := src.Push(ctx, layer1, bytes.NewReader(content1)); err != nil {
		t.Fatalf("Push() layer1 error = %v", err)
	}
	if err := src.Push(ctx, layer2, bytes.NewReader(content2)); err != nil {
		t.Fatalf("Push() layer2 error = %v", err)
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageConfig,
			Digest:    ocispec.DescriptorEmptyJSON.Digest,
			Size:      ocispec.DescriptorEmptyJSON.Size,
		},
		Layers: []ocispec.Descriptor{
			layer1,
			layer2,
		},
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}

	if err := src.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		t.Fatalf("Push() manifest error = %v", err)
	}
	if err := src.Tag(ctx, manifestDesc, "test"); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}

	cleanup, err := newPullCleanup(output)
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	statusHandler := status.NewDiscardHandler()
	metadataHandler := metadatajson.NewPullHandler(&bytes.Buffer{}, output)

	po := &pullOptions{
		Target: option.Target{
			Reference: "test",
		},
		Output: output,
	}

	_, err = doPull(
		ctx,
		src,
		dst,
		oras.CopyOptions{},
		metadataHandler,
		statusHandler,
		po,
		cleanup,
	)
	if !errors.Is(err, file.ErrDuplicateName) {
		t.Fatalf("doPull() error = %v, want duplicate-name error", err)
	}

	if _, ok := cleanup.created[filepath.Join(output, filePath)]; !ok {
		t.Fatalf("cleanup did not track %q", filePath)
	}
	if cleanupErr := cleanup.cleanup(); cleanupErr != nil {
		t.Fatalf("cleanup() error = %v", cleanupErr)
	}

	if _, err := os.Stat(filepath.Join(output, filePath)); !os.IsNotExist(err) {
		t.Fatalf("partial output exists, want it removed")
	}
}

func TestPullCleanup_PreservesExistingFile(t *testing.T) {
	output := t.TempDir()
	filePath := "tool"
	path := filepath.Join(output, filePath)

	if err := os.WriteFile(path, []byte("existing content\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cleanup, err := newPullCleanup(output)
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	cleanup.track(filePath)

	if _, ok := cleanup.created[path]; ok {
		t.Fatalf("cleanup tracked pre-existing file %q", filePath)
	}

	if err := cleanup.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != "existing content\n" {
		t.Fatalf("existing file changed: got %q", got)
	}
}

func TestPullCleanup_TrackRoot(t *testing.T) {
	cleanup, err := newPullCleanup(t.TempDir())
	if err != nil {
		t.Fatalf("newPullCleanup() error = %v", err)
	}

	cleanup.track(".")

	if len(cleanup.created) != 0 {
		t.Fatalf("cleanup.created = %v, want empty", cleanup.created)
	}
}
