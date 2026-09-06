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

// StopTrackTargetFunc is the function type to stop tracking a target.
type StopTrackTargetFunc func() error

// PushHandler handles status output for push command.
type PushHandler interface {
	OnFileLoading(name string) error
	OnEmptyArtifact() error
	TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error)
	OnCopySkipped(ctx context.Context, desc ocispec.Descriptor) error
	PreCopy(ctx context.Context, desc ocispec.Descriptor) error
	PostCopy(ctx context.Context, desc ocispec.Descriptor) error
}

// AttachHandler handles text status output for attach command.
type AttachHandler PushHandler

// PullHandler handles status output for pull command.
type PullHandler interface {
	// TrackTarget returns a tracked target.
	// If no TTY is available, it returns the original target.
	TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error)
	// OnNodeProcessing is called when processing a manifest.
	OnNodeProcessing(desc ocispec.Descriptor) error
	// OnNodeDownloading is called before downloading a node.
	OnNodeDownloading(desc ocispec.Descriptor) error
	// OnNodeDownloaded is called after a node is downloaded.
	OnNodeDownloaded(desc ocispec.Descriptor) error
	// OnNodeRestored is called after a deduplicated node is restored.
	OnNodeRestored(desc ocispec.Descriptor) error
	// OnNodeSkipped is called when a node is skipped.
	OnNodeSkipped(desc ocispec.Descriptor) error
}

// CopyHandler handles status output for cp command.
type CopyHandler interface {
	OnCopySkipped(ctx context.Context, desc ocispec.Descriptor) error
	PreCopy(ctx context.Context, desc ocispec.Descriptor) error
	PostCopy(ctx context.Context, desc ocispec.Descriptor) error
	OnMounted(ctx context.Context, desc ocispec.Descriptor) error
	StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error)
	StopTracking() error
}

// BackupHandler handles status output for backup command.
type BackupHandler interface {
	StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error)
	StopTracking() error
	OnCopySkipped(ctx context.Context, desc ocispec.Descriptor) error
	PreCopy(ctx context.Context, desc ocispec.Descriptor) error
	PostCopy(ctx context.Context, desc ocispec.Descriptor) error
}

// RestoreHandler handles status output for restore command.
type RestoreHandler interface {
	StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error)
	StopTracking() error
	OnCopySkipped(ctx context.Context, desc ocispec.Descriptor) error
	PreCopy(ctx context.Context, desc ocispec.Descriptor) error
	PostCopy(ctx context.Context, desc ocispec.Descriptor) error
}

// ManifestPushHandler handles status output for manifest push command.
type ManifestPushHandler interface {
	OnManifestPushSkipped() error
	OnManifestPushing() error
	OnManifestPushed() error
}

// ManifestIndexCreateHandler handles status output for manifest index create command.
type ManifestIndexCreateHandler interface {
	OnFetching(manifestRef string) error
	OnFetched(manifestRef string, desc ocispec.Descriptor) error
	OnIndexPacked(desc ocispec.Descriptor) error
	OnIndexPushed(path string) error
}

// ManifestIndexUpdateHandler handles status output for manifest index update command.
type ManifestIndexUpdateHandler interface {
	ManifestIndexCreateHandler
	OnManifestRemoved(digest digest.Digest) error
	OnManifestAdded(manifestRef string, desc ocispec.Descriptor) error
	OnIndexMerged(indexRef string, desc ocispec.Descriptor) error
}

// BlobPushHandler handles status output for blob push command.
type BlobPushHandler interface {
	OnBlobExists() error
	OnBlobUploading() error
	OnBlobUploaded() error
	StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error)
	StopTracking() error
}
