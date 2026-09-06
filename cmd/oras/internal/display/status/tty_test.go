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

package status

import (
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/internal/progress"
	"oras.land/oras/internal/testutils"
)

func TestTTYPushHandler_OnFileLoading(t *testing.T) {
	ph := NewTTYPushHandler(os.Stdout, mockFetcher.Fetcher)
	if ph.OnFileLoading("test") != nil {
		t.Error("OnFileLoading() should not return an error")
	}
}

func TestTTYPushHandler_OnEmptyArtifact(t *testing.T) {
	ph := NewTTYAttachHandler(os.Stdout, mockFetcher.Fetcher)
	if ph.OnEmptyArtifact() != nil {
		t.Error("OnEmptyArtifact() should not return an error")
	}
}

func TestTTYPushHandler_TrackTarget_invalidTTY(t *testing.T) {
	ph := NewTTYPushHandler(nil, mockFetcher.Fetcher)
	if _, _, err := ph.TrackTarget(nil); err == nil {
		t.Error("TrackTarget() should return an error for nil tty")
	}
}

func TestTTYPullHandler_OnNodeDownloading(t *testing.T) {
	ph := NewTTYPullHandler(nil)
	if err := ph.OnNodeDownloading(ocispec.Descriptor{}); err != nil {
		t.Error("OnNodeDownloading() should not return an error")
	}
}

func TestTTYPullHandler_OnNodeDownloaded(t *testing.T) {
	ph := NewTTYPullHandler(nil)
	if err := ph.OnNodeDownloaded(ocispec.Descriptor{}); err != nil {
		t.Error("OnNodeDownloaded() should not return an error")
	}
}

func TestTTYPullHandler_OnNodeProcessing(t *testing.T) {
	ph := NewTTYPullHandler(nil)
	if err := ph.OnNodeProcessing(ocispec.Descriptor{}); err != nil {
		t.Error("OnNodeProcessing() should not return an error")
	}
}

func TestTTYPushHandler_PostCopy(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle])
	ph := &TTYPushHandler{
		tracked:   &testutils.PromptDiscarder{},
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := ph.PostCopy(ctx, fetcher.OciImage); err != nil {
		t.Errorf("unexpected error from PostCopy(): %v", err)
	}
}

func TestTTYPushHandler_PostCopy_errGetSuccessor(t *testing.T) {
	errorFetcher := testutils.NewErrorFetcher()
	ph := NewTTYPushHandler(nil, errorFetcher)
	err := ph.PostCopy(ctx, ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	})
	if err.Error() != errorFetcher.ExpectedError.Error() {
		t.Errorf("PostCopy() should return expected error got %v", err.Error())
	}
}

func TestTTYPushHandler_PostCopy_errPrompt(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"1")
	wantedError := errors.New("wanted error")
	ph := &TTYPushHandler{
		tracked:   testutils.NewErrorPrompt(wantedError),
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := ph.PostCopy(ctx, fetcher.OciImage); !errors.Is(err, wantedError) {
		t.Errorf("PostCopy() should return expected error got %v", err)
	}
}

func TestNewTTYBackupHandler(t *testing.T) {
	handler := NewTTYBackupHandler(os.Stdout, nil)
	if handler == nil {
		t.Error("NewTTYBackupHandler() should not return nil")
	}
}

func TestTTYBackupHandler_StartTracking_invalidTTY(t *testing.T) {
	bh := NewTTYBackupHandler(nil, nil)
	gt := memory.New()
	if _, err := bh.StartTracking(gt); err == nil {
		t.Error("StartTracking() should return an error for nil tty")
	}
}

func TestTTYBackupHandler_OnCopySkipped(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	bh := &TTYBackupHandler{
		tracked:   &testutils.PromptDiscarder{}, // Keep PromptDiscarder here for Report method
		committed: &sync.Map{},
		fetcher:   fetcher.Fetcher,
	}
	if err := bh.OnCopySkipped(ctx, fetcher.ImageLayer); err != nil {
		t.Errorf("OnCopySkipped() should not return an error: %v", err)
	}

	// Verify that the descriptor is stored in the committed map
	if _, ok := bh.committed.Load(fetcher.ImageLayer.Digest.String()); !ok {
		t.Error("OnCopySkipped() should store the descriptor in the committed map")
	}
}

func TestTTYBackupHandler_PreCopy(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	bh := &TTYBackupHandler{}
	if err := bh.PreCopy(ctx, fetcher.ImageLayer); err != nil {
		t.Errorf("PreCopy() should not return an error: %v", err)
	}
}

func TestTTYBackupHandler_PostCopy(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle])
	bh := &TTYBackupHandler{
		tracked:   &testutils.PromptDiscarder{},
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := bh.PostCopy(ctx, fetcher.OciImage); err != nil {
		t.Errorf("unexpected error from PostCopy(): %v", err)
	}
}

func TestTTYBackupHandler_PostCopy_errGetSuccessor(t *testing.T) {
	errorFetcher := testutils.NewErrorFetcher()
	prompt := &testutils.PromptDiscarder{}
	bh := &TTYBackupHandler{
		tracked:   prompt,
		committed: &sync.Map{},
		fetcher:   errorFetcher,
	}

	err := bh.PostCopy(ctx, ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	})

	if err == nil || err.Error() != errorFetcher.ExpectedError.Error() {
		t.Errorf("PostCopy() should return expected error got %v", err.Error())
	}
}

func TestTTYBackupHandler_PostCopy_errPrompt(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"1")
	wantedError := errors.New("wanted error")
	bh := &TTYBackupHandler{
		tracked:   testutils.NewErrorPrompt(wantedError),
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := bh.PostCopy(ctx, fetcher.OciImage); !errors.Is(err, wantedError) {
		t.Errorf("PostCopy() should return expected error got %v", err)
	}
}

func TestNewTTYRestoreHandler(t *testing.T) {
	handler := NewTTYRestoreHandler(os.Stdout, nil)
	if handler == nil {
		t.Error("NewTTYRestoreHandler() should not return nil")
	}
}

func TestTTYRestoreHandler_StartTracking_invalidTTY(t *testing.T) {
	rh := NewTTYRestoreHandler(nil, nil)
	gt := memory.New()
	if _, err := rh.StartTracking(gt); err == nil {
		t.Error("StartTracking() should return an error for nil tty")
	}
}

func TestTTYRestoreHandler_OnCopySkipped(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	rh := &TTYRestoreHandler{
		tracked:   &testutils.PromptDiscarder{},
		committed: &sync.Map{},
		fetcher:   fetcher.Fetcher,
	}
	if err := rh.OnCopySkipped(ctx, fetcher.ImageLayer); err != nil {
		t.Errorf("OnCopySkipped() should not return an error: %v", err)
	}

	// Verify that the descriptor is stored in the committed map
	if _, ok := rh.committed.Load(fetcher.ImageLayer.Digest.String()); !ok {
		t.Error("OnCopySkipped() should store the descriptor in the committed map")
	}
}

func TestTTYRestoreHandler_PreCopy(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	rh := &TTYRestoreHandler{}
	if err := rh.PreCopy(ctx, fetcher.ImageLayer); err != nil {
		t.Errorf("PreCopy() should not return an error: %v", err)
	}
}

func TestTTYRestoreHandler_PostCopy(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle])
	rh := &TTYRestoreHandler{
		tracked:   &testutils.PromptDiscarder{},
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := rh.PostCopy(ctx, fetcher.OciImage); err != nil {
		t.Errorf("unexpected error from PostCopy(): %v", err)
	}
}

func TestTTYRestoreHandler_PostCopy_errGetSuccessor(t *testing.T) {
	errorFetcher := testutils.NewErrorFetcher()
	prompt := &testutils.PromptDiscarder{}
	rh := &TTYRestoreHandler{
		tracked:   prompt,
		committed: &sync.Map{},
		fetcher:   errorFetcher,
	}

	err := rh.PostCopy(ctx, ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	})

	if err == nil || err.Error() != errorFetcher.ExpectedError.Error() {
		t.Errorf("PostCopy() should return expected error got %v", err)
	}
}

func TestTTYRestoreHandler_PostCopy_errPrompt(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	committed := &sync.Map{}
	committed.Store(fetcher.ImageLayer.Digest.String(), fetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"1")
	wantedError := errors.New("wanted error")
	rh := &TTYRestoreHandler{
		tracked:   testutils.NewErrorPrompt(wantedError),
		committed: committed,
		fetcher:   fetcher.Fetcher,
	}
	if err := rh.PostCopy(ctx, fetcher.OciImage); !errors.Is(err, wantedError) {
		t.Errorf("PostCopy() should return expected error got %v", err)
	}
}

// recordingTrackedTarget records the progress states reported to a tracked
// target and whether it was closed.
type recordingTrackedTarget struct {
	oras.GraphTarget
	states   []progress.State
	closed   bool
	closeErr error
}

func (r *recordingTrackedTarget) Report(_ ocispec.Descriptor, state progress.State) error {
	r.states = append(r.states, state)
	return nil
}

func (r *recordingTrackedTarget) Close() error {
	r.closed = true
	return r.closeErr
}

func TestTTYPushHandler_OnCopySkipped(t *testing.T) {
	fetcher := testutils.NewMockFetcher()
	tracked := &recordingTrackedTarget{}
	ph := &TTYPushHandler{
		tracked:   tracked,
		committed: &sync.Map{},
		fetcher:   fetcher.Fetcher,
	}
	if err := ph.OnCopySkipped(ctx, fetcher.ImageLayer); err != nil {
		t.Fatalf("OnCopySkipped() error = %v", err)
	}
	if want := []progress.State{progress.StateExists}; !reflect.DeepEqual(tracked.states, want) {
		t.Errorf("reported states = %v, want %v", tracked.states, want)
	}
	if _, ok := ph.committed.Load(fetcher.ImageLayer.Digest.String()); !ok {
		t.Error("OnCopySkipped() should store the descriptor in the committed map")
	}
}

func TestTTYPushHandler_PreCopy(t *testing.T) {
	tracked := &recordingTrackedTarget{}
	ph := &TTYPushHandler{tracked: tracked, committed: &sync.Map{}}
	if err := ph.PreCopy(ctx, mockFetcher.OciImage); err != nil {
		t.Fatalf("PreCopy() error = %v", err)
	}
	if len(tracked.states) != 0 {
		t.Errorf("PreCopy() reported states %v, want none", tracked.states)
	}
}

func TestTTYPullHandler_OnNodeRestoredAndSkipped(t *testing.T) {
	tracked := &recordingTrackedTarget{}
	ph := &TTYPullHandler{tracked: tracked}
	if err := ph.OnNodeRestored(mockFetcher.OciImage); err != nil {
		t.Fatalf("OnNodeRestored() error = %v", err)
	}
	if err := ph.OnNodeSkipped(mockFetcher.OciImage); err != nil {
		t.Fatalf("OnNodeSkipped() error = %v", err)
	}
	if want := []progress.State{progress.StateRestored, progress.StateSkipped}; !reflect.DeepEqual(tracked.states, want) {
		t.Errorf("reported states = %v, want %v", tracked.states, want)
	}
}

func TestTTYBackupHandler_StopTracking(t *testing.T) {
	wantErr := errors.New("close failed")
	tracked := &recordingTrackedTarget{closeErr: wantErr}
	bh := &TTYBackupHandler{tracked: tracked}
	if err := bh.StopTracking(); !errors.Is(err, wantErr) {
		t.Fatalf("StopTracking() error = %v, want %v", err, wantErr)
	}
	if !tracked.closed {
		t.Error("StopTracking() should close the tracked target")
	}
}

func TestTTYRestoreHandler_StopTracking(t *testing.T) {
	wantErr := errors.New("close failed")
	tracked := &recordingTrackedTarget{closeErr: wantErr}
	rh := &TTYRestoreHandler{tracked: tracked}
	if err := rh.StopTracking(); !errors.Is(err, wantErr) {
		t.Fatalf("StopTracking() error = %v, want %v", err, wantErr)
	}
	if !tracked.closed {
		t.Error("StopTracking() should close the tracked target")
	}
}

func TestTTYBlobPushHandler_StartTracking_invalidTTY(t *testing.T) {
	bh := NewTTYBlobPushHandler(nil, mockFetcher.OciImage)
	if _, err := bh.StartTracking(memory.New()); err == nil {
		t.Error("StartTracking() should return an error for nil tty")
	}
}

func TestTTYBlobPushHandler_reportsStates(t *testing.T) {
	tracked := &recordingTrackedTarget{}
	bh := &TTYBlobPushHandler{desc: mockFetcher.OciImage, tracked: tracked}
	if err := bh.OnBlobUploading(); err != nil {
		t.Fatalf("OnBlobUploading() error = %v", err)
	}
	if err := bh.OnBlobUploaded(); err != nil {
		t.Fatalf("OnBlobUploaded() error = %v", err)
	}
	if err := bh.OnBlobExists(); err != nil {
		t.Fatalf("OnBlobExists() error = %v", err)
	}
	// uploading and uploaded progress comes from the tracked target itself;
	// only an existing blob is reported explicitly.
	if want := []progress.State{progress.StateExists}; !reflect.DeepEqual(tracked.states, want) {
		t.Errorf("reported states = %v, want %v", tracked.states, want)
	}
}
