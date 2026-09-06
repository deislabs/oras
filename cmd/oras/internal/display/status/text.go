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
	"sync"

	"oras.land/oras/internal/contentutil"
	"oras.land/oras/internal/descriptor"
	"oras.land/oras/internal/graph"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"oras.land/oras/cmd/oras/internal/output"
)

// TextPushHandler handles text status output for push events.
type TextPushHandler struct {
	printer   *output.Printer
	committed *sync.Map
	fetcher   content.Fetcher
}

// NewTextPushHandler returns a new handler for push command.
func NewTextPushHandler(printer *output.Printer, fetcher content.Fetcher) PushHandler {
	tch := TextPushHandler{
		printer:   printer,
		fetcher:   fetcher,
		committed: &sync.Map{},
	}
	return &tch
}

// OnFileLoading is called when a file is being prepared for upload.
func (ph *TextPushHandler) OnFileLoading(name string) error {
	return ph.printer.PrintVerbose("Preparing", name)
}

// OnEmptyArtifact is called when an empty artifact is being uploaded.
func (ph *TextPushHandler) OnEmptyArtifact() error {
	return ph.printer.Println("Uploading empty artifact")
}

// TrackTarget returns a tracked target.
func (ph *TextPushHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error) {
	return gt, discardStopTrack, nil
}

// OnCopySkipped is called when an object already exists.
func (ph *TextPushHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	ph.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ph.printer.PrintStatus(desc, PushPromptExists)
}

// PreCopy implements PreCopy of CopyHandler.
func (ph *TextPushHandler) PreCopy(_ context.Context, desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PushPromptUploading)
}

// PostCopy implements PostCopy of CopyHandler.
func (ph *TextPushHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	ph.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	successors, err := graph.FilteredSuccessors(ctx, desc, ph.fetcher, DeduplicatedFilter(ph.committed))
	if err != nil {
		return err
	}
	for _, successor := range successors {
		if err = ph.printer.PrintStatus(successor, PushPromptExists); err != nil {
			return err
		}
	}
	return ph.printer.PrintStatus(desc, PushPromptUploaded)
}

// NewTextAttachHandler returns a new handler for attach command.
func NewTextAttachHandler(printer *output.Printer, fetcher content.Fetcher) AttachHandler {
	return NewTextPushHandler(printer, fetcher)
}

// TextPullHandler handles text status output for pull events.
type TextPullHandler struct {
	printer *output.Printer
}

// TrackTarget implements PullHandler.
func (ph *TextPullHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error) {
	return gt, discardStopTrack, nil
}

// OnNodeDownloading implements PullHandler.
func (ph *TextPullHandler) OnNodeDownloading(desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PullPromptDownloading)
}

// OnNodeDownloaded implements PullHandler.
func (ph *TextPullHandler) OnNodeDownloaded(desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PullPromptDownloaded)
}

// OnNodeRestored implements PullHandler.
func (ph *TextPullHandler) OnNodeRestored(desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PullPromptRestored)
}

// OnNodeProcessing implements PullHandler.
func (ph *TextPullHandler) OnNodeProcessing(desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PullPromptProcessing)
}

// OnNodeSkipped implements PullHandler.
func (ph *TextPullHandler) OnNodeSkipped(desc ocispec.Descriptor) error {
	return ph.printer.PrintStatus(desc, PullPromptSkipped)
}

// NewTextPullHandler returns a new handler for pull command.
func NewTextPullHandler(printer *output.Printer) PullHandler {
	return &TextPullHandler{
		printer: printer,
	}
}

// TextCopyHandler handles text status output for push events.
type TextCopyHandler struct {
	printer   *output.Printer
	committed *sync.Map
	fetcher   content.Fetcher
}

// NewTextCopyHandler returns a new handler for push command.
func NewTextCopyHandler(printer *output.Printer, fetcher content.Fetcher) CopyHandler {
	return &TextCopyHandler{
		printer:   printer,
		fetcher:   fetcher,
		committed: &sync.Map{},
	}
}

// StartTracking starts a tracked target from a graph target.
func (ch *TextCopyHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	return gt, nil
}

// StopTracking ends the copy tracking for the target.
func (ch *TextCopyHandler) StopTracking() error {
	return nil
}

// OnCopySkipped is called when an object already exists.
func (ch *TextCopyHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ch.printer.PrintStatus(desc, copyPromptExists)
}

// PreCopy implements PreCopy of CopyHandler.
func (ch *TextCopyHandler) PreCopy(_ context.Context, desc ocispec.Descriptor) error {
	return ch.printer.PrintStatus(desc, copyPromptCopying)
}

// PostCopy implements PostCopy of CopyHandler.
func (ch *TextCopyHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	deduplicated, err := graph.FilteredSuccessors(ctx, desc, ch.fetcher, DeduplicatedFilter(ch.committed))
	if err != nil {
		return err
	}
	for _, successor := range deduplicated {
		if err = ch.printer.PrintStatus(successor, copyPromptSkipped); err != nil {
			return err
		}
	}
	return ch.printer.PrintStatus(desc, copyPromptCopied)
}

// OnMounted implements OnMounted of CopyHandler.
func (ch *TextCopyHandler) OnMounted(_ context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ch.printer.PrintStatus(desc, copyPromptMounted)
}

// TextBackupHandler handles text status output for backup events.
type TextBackupHandler struct {
	printer   *output.Printer
	committed *sync.Map
	fetcher   content.Fetcher
}

// NewTextBackupHandler returns a new text handler for backup command.
func NewTextBackupHandler(printer *output.Printer, fetcher content.Fetcher) BackupHandler {
	return &TextBackupHandler{
		printer:   printer,
		fetcher:   fetcher,
		committed: &sync.Map{},
	}
}

// OnCopySkipped implements OnCopySkipped of BackupHandler.
func (tbh *TextBackupHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	tbh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return tbh.printer.PrintStatus(desc, backupPromptExists)
}

// PreCopy implements PreCopy of BackupHandler.
func (tbh *TextBackupHandler) PreCopy(_ context.Context, desc ocispec.Descriptor) error {
	return tbh.printer.PrintStatus(desc, backupPromptPulling)
}

// PostCopy implements PostCopy of BackupHandler.
func (tbh *TextBackupHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	tbh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	deduplicated, err := graph.FilteredSuccessors(ctx, desc, tbh.fetcher, DeduplicatedFilter(tbh.committed))
	if err != nil {
		return err
	}
	for _, successor := range deduplicated {
		if err = tbh.printer.PrintStatus(successor, backupPromptSkipped); err != nil {
			return err
		}
	}
	return tbh.printer.PrintStatus(desc, backupPromptPulled)
}

// StartTracking implements StartTracking of BackupHandler.
func (tbh *TextBackupHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	return gt, nil
}

// StopTracking implements StopTracking of BackupHandler.
func (tbh *TextBackupHandler) StopTracking() error {
	return nil
}

// TextRestoreHandler handles text status output for restore events.
type TextRestoreHandler struct {
	printer   *output.Printer
	committed *sync.Map
	fetcher   content.Fetcher
}

// NewTextRestoreHandler returns a new text handler for restore command.
func NewTextRestoreHandler(printer *output.Printer, fetcher content.Fetcher) RestoreHandler {
	return &TextRestoreHandler{
		printer:   printer,
		fetcher:   fetcher,
		committed: &sync.Map{},
	}
}

// OnCopySkipped implements OnCopySkipped of RestoreHandler.
func (trh *TextRestoreHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	trh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return trh.printer.PrintStatus(desc, restorePromptExists)
}

// PreCopy implements PreCopy of RestoreHandler.
func (trh *TextRestoreHandler) PreCopy(_ context.Context, desc ocispec.Descriptor) error {
	return trh.printer.PrintStatus(desc, restorePromptPushing)
}

// PostCopy implements PostCopy of RestoreHandler.
func (trh *TextRestoreHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	trh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	deduplicated, err := graph.FilteredSuccessors(ctx, desc, trh.fetcher, DeduplicatedFilter(trh.committed))
	if err != nil {
		return err
	}
	for _, successor := range deduplicated {
		if err = trh.printer.PrintStatus(successor, restorePromptSkipped); err != nil {
			return err
		}
	}
	return trh.printer.PrintStatus(desc, restorePromptPushed)
}

// StartTracking implements StartTracking of RestoreHandler.
func (trh *TextRestoreHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	return gt, nil
}

// StopTracking implements StopTracking of RestoreHandler.
func (trh *TextRestoreHandler) StopTracking() error {
	return nil
}

// TextManifestPushHandler handles text status output for manifest push events.
type TextManifestPushHandler struct {
	desc    ocispec.Descriptor
	printer *output.Printer
}

// NewTextManifestPushHandler returns a new handler for manifest push command.
func NewTextManifestPushHandler(printer *output.Printer, desc ocispec.Descriptor) ManifestPushHandler {
	return &TextManifestPushHandler{
		desc:    desc,
		printer: printer,
	}
}

func (mph *TextManifestPushHandler) OnManifestPushSkipped() error {
	return mph.printer.PrintStatus(mph.desc, PushPromptExists)
}

func (mph *TextManifestPushHandler) OnManifestPushing() error {
	return mph.printer.PrintStatus(mph.desc, PushPromptUploading)
}

func (mph *TextManifestPushHandler) OnManifestPushed() error {
	return mph.printer.PrintStatus(mph.desc, PushPromptUploaded)
}

// TextManifestIndexCreateHandler handles text status output for manifest index create events.
type TextManifestIndexCreateHandler struct {
	printer *output.Printer
}

// NewTextManifestIndexCreateHandler returns a new handler for manifest index create command.
func NewTextManifestIndexCreateHandler(printer *output.Printer) ManifestIndexCreateHandler {
	tmich := TextManifestIndexCreateHandler{
		printer: printer,
	}
	return &tmich
}

// OnFetching implements ManifestIndexCreateHandler.
func (mich *TextManifestIndexCreateHandler) OnFetching(source string) error {
	return mich.printer.Println(IndexPromptFetching, source)
}

// OnFetched implements ManifestIndexCreateHandler.
func (mich *TextManifestIndexCreateHandler) OnFetched(ref string, desc ocispec.Descriptor) error {
	if contentutil.IsDigest(ref) {
		return mich.printer.Println(IndexPromptFetched, ref)
	}
	return mich.printer.Println(IndexPromptFetched, desc.Digest, ref)
}

// OnIndexPacked implements ManifestIndexCreateHandler.
func (mich *TextManifestIndexCreateHandler) OnIndexPacked(desc ocispec.Descriptor) error {
	return mich.printer.Println(IndexPromptPacked, descriptor.ShortDigest(desc), ocispec.MediaTypeImageIndex)
}

// OnIndexPushed implements ManifestIndexCreateHandler.
func (mich *TextManifestIndexCreateHandler) OnIndexPushed(path string) error {
	return mich.printer.Println(IndexPromptPushed, path)
}

// TextManifestIndexUpdateHandler handles text status output for manifest index update events.
type TextManifestIndexUpdateHandler struct {
	printer *output.Printer
}

// NewTextManifestIndexUpdateHandler returns a new handler for manifest index create command.
func NewTextManifestIndexUpdateHandler(printer *output.Printer) ManifestIndexUpdateHandler {
	miuh := TextManifestIndexUpdateHandler{
		printer: printer,
	}
	return &miuh
}

// OnFetching implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnFetching(ref string) error {
	return miuh.printer.Println(IndexPromptFetching, ref)
}

// OnFetched implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnFetched(ref string, desc ocispec.Descriptor) error {
	if contentutil.IsDigest(ref) {
		return miuh.printer.Println(IndexPromptFetched, ref)
	}
	return miuh.printer.Println(IndexPromptFetched, desc.Digest, ref)
}

// OnManifestRemoved implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnManifestRemoved(digest digest.Digest) error {
	return miuh.printer.Println(IndexPromptRemoved, digest)
}

// OnManifestAdded implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnManifestAdded(ref string, desc ocispec.Descriptor) error {
	if contentutil.IsDigest(ref) {
		return miuh.printer.Println(IndexPromptAdded, ref)
	}
	return miuh.printer.Println(IndexPromptAdded, desc.Digest, ref)
}

// OnIndexMerged implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnIndexMerged(ref string, desc ocispec.Descriptor) error {
	if contentutil.IsDigest(ref) {
		return miuh.printer.Println(IndexPromptMerged, ref)
	}
	return miuh.printer.Println(IndexPromptMerged, desc.Digest, ref)
}

// OnIndexPacked implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnIndexPacked(desc ocispec.Descriptor) error {
	return miuh.printer.Println(IndexPromptUpdated, desc.Digest)
}

// OnIndexPushed implements ManifestIndexUpdateHandler.
func (miuh *TextManifestIndexUpdateHandler) OnIndexPushed(indexRef string) error {
	return miuh.printer.Println(IndexPromptPushed, indexRef)
}

// TextBlobPushHandler handles text status output for blob push events.
type TextBlobPushHandler struct {
	desc    ocispec.Descriptor
	printer *output.Printer
}

// NewTextBlobPushHandler returns a new handler for blob push command.
func NewTextBlobPushHandler(printer *output.Printer, desc ocispec.Descriptor) BlobPushHandler {
	return &TextBlobPushHandler{
		desc:    desc,
		printer: printer,
	}
}

// OnBlobExists implements BlobPushHandler.
func (bph *TextBlobPushHandler) OnBlobExists() error {
	return bph.printer.PrintStatus(bph.desc, PushPromptExists)
}

// OnBlobUploading implements BlobPushHandler.
func (bph *TextBlobPushHandler) OnBlobUploading() error {
	return bph.printer.PrintStatus(bph.desc, PushPromptUploading)
}

// OnBlobUploaded implements BlobPushHandler.
func (bph *TextBlobPushHandler) OnBlobUploaded() error {
	return bph.printer.PrintStatus(bph.desc, PushPromptUploaded)
}

// StartTracking implements BlobPushHandler.
func (bph *TextBlobPushHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	return gt, nil
}

// StopTracking implements BlobPushHandler.
func (bph *TextBlobPushHandler) StopTracking() error {
	return nil
}
