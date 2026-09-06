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

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
)

func discardStopTrack() error {
	return nil
}

// DiscardHandler is a no-op handler that discards all status updates.
type DiscardHandler struct{}

// NewDiscardHandler returns a new no-op handler.
func NewDiscardHandler() DiscardHandler {
	return DiscardHandler{}
}

// OnFileLoading is called before a file is being loaded.
func (DiscardHandler) OnFileLoading(string) error {
	return nil
}

// OnEmptyArtifact is called when no file is loaded for an artifact push.
func (DiscardHandler) OnEmptyArtifact() error {
	return nil
}

// TrackTarget returns a target with status tracking.
func (DiscardHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error) {
	return gt, discardStopTrack, nil
}

// OnCopySkipped is called when an object already exists.
func (DiscardHandler) OnCopySkipped(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// PreCopy implements PreCopy of CopyHandler.
func (DiscardHandler) PreCopy(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// PostCopy implements PostCopy of CopyHandler.
func (DiscardHandler) PostCopy(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// OnNodeDownloading implements PullHandler.
func (DiscardHandler) OnNodeDownloading(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeDownloaded implements PullHandler.
func (DiscardHandler) OnNodeDownloaded(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeRestored implements PullHandler.
func (DiscardHandler) OnNodeRestored(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeProcessing implements PullHandler.
func (DiscardHandler) OnNodeProcessing(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeProcessing implements PullHandler.
func (DiscardHandler) OnNodeSkipped(_ ocispec.Descriptor) error {
	return nil
}

// OnFetching implements referenceFetchHandler.
func (DiscardHandler) OnFetching(string) error {
	return nil
}

// OnFetched implements referenceFetchHandler.
func (DiscardHandler) OnFetched(string, ocispec.Descriptor) error {
	return nil
}

// OnManifestPushSkipped implements ManifestPushHandler.
func (DiscardHandler) OnManifestPushSkipped() error {
	return nil
}

// OnManifestPushing implements ManifestPushHandler.
func (DiscardHandler) OnManifestPushing() error {
	return nil
}

// OnManifestPushed implements ManifestPushHandler.
func (DiscardHandler) OnManifestPushed() error {
	return nil
}

// OnManifestRemoved implements ManifestIndexUpdateHandler.
func (DiscardHandler) OnManifestRemoved(digest.Digest) error {
	return nil
}

// OnManifestAdded implements ManifestIndexUpdateHandler.
func (DiscardHandler) OnManifestAdded(string, ocispec.Descriptor) error {
	return nil
}

// OnIndexMerged implements ManifestIndexUpdateHandler.
func (DiscardHandler) OnIndexMerged(string, ocispec.Descriptor) error {
	return nil
}

// OnIndexPacked implements ManifestIndexCreateHandler.
func (DiscardHandler) OnIndexPacked(ocispec.Descriptor) error {
	return nil
}

// OnIndexPushed implements ManifestIndexCreateHandler.
func (DiscardHandler) OnIndexPushed(string) error {
	return nil
}

// OnBlobExists implements BlobPushHandler.
func (DiscardHandler) OnBlobExists() error {
	return nil
}

// OnBlobUploading implements BlobPushHandler.
func (DiscardHandler) OnBlobUploading() error {
	return nil
}

// OnBlobUploaded implements BlobPushHandler.
func (DiscardHandler) OnBlobUploaded() error {
	return nil
}

// StartTracking implements BlobPushHandler.
func (DiscardHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	return gt, nil
}

// StopTracking implements BlobPushHandler.
func (DiscardHandler) StopTracking() error {
	return nil
}
