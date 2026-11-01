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
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestDiscardHandler_OnPushSkipped(t *testing.T) {
	testDiscard := NewDiscardHandler()
	if err := testDiscard.OnManifestPushSkipped(); err != nil {
		t.Errorf("DiscardHandler.OnPushSkipped() error = %v, wantErr nil", err)
	}
}

func TestDiscardHandler_OnManifestRemoved(t *testing.T) {
	testDiscard := NewDiscardHandler()
	if err := testDiscard.OnManifestRemoved("sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"); err != nil {
		t.Errorf("DiscardHandler.OnManifestRemoved() error = %v, wantErr nil", err)
	}
}

func TestDiscardHandler_OnIndexMerged(t *testing.T) {
	testDiscard := NewDiscardHandler()
	if err := testDiscard.OnIndexMerged("test", v1.Descriptor{}); err != nil {
		t.Errorf("DiscardHandler.OnIndexMerged() error = %v, wantErr nil", err)
	}
}

func TestDiscardHandler_OnIndexPushed(t *testing.T) {
	testDiscard := NewDiscardHandler()
	if err := testDiscard.OnIndexPushed("test"); err != nil {
		t.Errorf("DiscardHandler.OnIndexPushed() error = %v, wantErr nil", err)
	}
}

func TestDiscardHandler_AllMethods(t *testing.T) {
	testDiscard := NewDiscardHandler()
	desc := v1.Descriptor{}
	ctx := context.Background()

	if err := testDiscard.OnFileLoading("test"); err != nil {
		t.Errorf("OnFileLoading() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnEmptyArtifact(); err != nil {
		t.Errorf("OnEmptyArtifact() error = %v, wantErr nil", err)
	}
	if got, stop, err := testDiscard.TrackTarget(nil); err != nil || got != nil || stop() != nil {
		t.Errorf("TrackTarget() = (%v, %v, %v), want (nil, non-nil, nil)", got, stop, err)
	}
	if err := testDiscard.OnCopySkipped(ctx, desc); err != nil {
		t.Errorf("OnCopySkipped() error = %v, wantErr nil", err)
	}
	if err := testDiscard.PreCopy(ctx, desc); err != nil {
		t.Errorf("PreCopy() error = %v, wantErr nil", err)
	}
	if err := testDiscard.PostCopy(ctx, desc); err != nil {
		t.Errorf("PostCopy() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnNodeDownloading(desc); err != nil {
		t.Errorf("OnNodeDownloading() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnNodeDownloaded(desc); err != nil {
		t.Errorf("OnNodeDownloaded() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnNodeRestored(desc); err != nil {
		t.Errorf("OnNodeRestored() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnNodeProcessing(desc); err != nil {
		t.Errorf("OnNodeProcessing() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnNodeSkipped(desc); err != nil {
		t.Errorf("OnNodeSkipped() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnFetching("ref"); err != nil {
		t.Errorf("OnFetching() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnFetched("ref", desc); err != nil {
		t.Errorf("OnFetched() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnManifestPushSkipped(); err != nil {
		t.Errorf("OnManifestPushSkipped() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnManifestPushing(); err != nil {
		t.Errorf("OnManifestPushing() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnManifestPushed(); err != nil {
		t.Errorf("OnManifestPushed() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnManifestRemoved(digest.Digest("sha256:test")); err != nil {
		t.Errorf("OnManifestRemoved() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnManifestAdded("ref", desc); err != nil {
		t.Errorf("OnManifestAdded() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnIndexMerged("ref", desc); err != nil {
		t.Errorf("OnIndexMerged() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnIndexPacked(desc); err != nil {
		t.Errorf("OnIndexPacked() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnIndexPushed("path"); err != nil {
		t.Errorf("OnIndexPushed() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnBlobExists(); err != nil {
		t.Errorf("OnBlobExists() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnBlobUploading(); err != nil {
		t.Errorf("OnBlobUploading() error = %v, wantErr nil", err)
	}
	if err := testDiscard.OnBlobUploaded(); err != nil {
		t.Errorf("OnBlobUploaded() error = %v, wantErr nil", err)
	}
	if got, err := testDiscard.StartTracking(nil); err != nil || got != nil {
		t.Errorf("StartTracking() = (%v, %v), want (nil, nil)", got, err)
	}
	if err := testDiscard.StopTracking(); err != nil {
		t.Errorf("StopTracking() error = %v, wantErr nil", err)
	}
}
