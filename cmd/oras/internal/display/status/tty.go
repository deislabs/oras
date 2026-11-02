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
	"os"
	"sync"

	"oras.land/oras/internal/graph"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"oras.land/oras/cmd/oras/internal/display/status/track"
	"oras.land/oras/internal/progress"
)

// TTYPushHandler handles TTY status output for push command.
type TTYPushHandler struct {
	tty       *os.File
	tracked   track.GraphTarget
	committed *sync.Map
	fetcher   content.Fetcher
}

// NewTTYPushHandler returns a new handler for push status events.
func NewTTYPushHandler(tty *os.File, fetcher content.Fetcher) PushHandler {
	return &TTYPushHandler{
		tty:       tty,
		fetcher:   fetcher,
		committed: &sync.Map{},
	}
}

// OnFileLoading is called before loading a file.
func (ph *TTYPushHandler) OnFileLoading(_ string) error {
	return nil
}

// OnEmptyArtifact is called when no file is loaded for an artifact push.
func (ph *TTYPushHandler) OnEmptyArtifact() error {
	return nil
}

// TrackTarget returns a tracked target.
func (ph *TTYPushHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error) {
	prompt := map[progress.State]string{
		progress.StateInitialized:  PushPromptUploading,
		progress.StateTransmitting: PushPromptUploading,
		progress.StateTransmitted:  PushPromptUploaded,
		progress.StateExists:       PushPromptExists,
		progress.StateSkipped:      PushPromptSkipped,
	}
	tracked, err := track.NewTarget(gt, prompt, ph.tty)
	if err != nil {
		return nil, nil, err
	}
	ph.tracked = tracked
	return tracked, tracked.Close, nil
}

// OnCopySkipped is called when an object already exists.
func (ph *TTYPushHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	ph.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ph.tracked.Report(desc, progress.StateExists)
}

// PreCopy implements PreCopy of CopyHandler.
func (ph *TTYPushHandler) PreCopy(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// PostCopy implements PostCopy of CopyHandler.
func (ph *TTYPushHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	ph.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	successors, err := graph.FilteredSuccessors(ctx, desc, ph.fetcher, DeduplicatedFilter(ph.committed))
	if err != nil {
		return err
	}
	for _, successor := range successors {
		if err = ph.tracked.Report(successor, progress.StateSkipped); err != nil {
			return err
		}
	}
	return nil
}

// NewTTYAttachHandler returns a new handler for attach status events.
func NewTTYAttachHandler(tty *os.File, fetcher content.Fetcher) AttachHandler {
	return NewTTYPushHandler(tty, fetcher)
}

// TTYPullHandler handles TTY status output for pull events.
type TTYPullHandler struct {
	tty     *os.File
	tracked track.GraphTarget
}

// NewTTYPullHandler returns a new handler for Pull status events.
func NewTTYPullHandler(tty *os.File) PullHandler {
	return &TTYPullHandler{
		tty: tty,
	}
}

// OnNodeDownloading implements PullHandler.
func (ph *TTYPullHandler) OnNodeDownloading(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeDownloaded implements PullHandler.
func (ph *TTYPullHandler) OnNodeDownloaded(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeProcessing implements PullHandler.
func (ph *TTYPullHandler) OnNodeProcessing(_ ocispec.Descriptor) error {
	return nil
}

// OnNodeRestored implements PullHandler.
func (ph *TTYPullHandler) OnNodeRestored(desc ocispec.Descriptor) error {
	return ph.tracked.Report(desc, progress.StateRestored)
}

// OnNodeSkipped implements PullHandler.
func (ph *TTYPullHandler) OnNodeSkipped(desc ocispec.Descriptor) error {
	return ph.tracked.Report(desc, progress.StateSkipped)
}

// TrackTarget returns a tracked target.
func (ph *TTYPullHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, StopTrackTargetFunc, error) {
	prompt := map[progress.State]string{
		progress.StateInitialized:  PullPromptDownloading,
		progress.StateTransmitting: PullPromptDownloading,
		progress.StateTransmitted:  PullPromptPulled,
		progress.StateSkipped:      PullPromptSkipped,
		progress.StateRestored:     PullPromptRestored,
	}
	tracked, err := track.NewTarget(gt, prompt, ph.tty)
	if err != nil {
		return nil, nil, err
	}
	ph.tracked = tracked
	return tracked, tracked.Close, nil
}

// TTYCopyHandler handles tty status output for copy events.
type TTYCopyHandler struct {
	tty       *os.File
	committed sync.Map
	tracked   track.GraphTarget
}

// NewTTYCopyHandler returns a new handler for copy command.
func NewTTYCopyHandler(tty *os.File) CopyHandler {
	return &TTYCopyHandler{
		tty: tty,
	}
}

// StartTracking returns a tracked target from a graph target.
func (ch *TTYCopyHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	prompt := map[progress.State]string{
		progress.StateInitialized:  copyPromptCopying,
		progress.StateTransmitting: copyPromptCopying,
		progress.StateTransmitted:  copyPromptCopied,
		progress.StateExists:       copyPromptExists,
		progress.StateSkipped:      copyPromptSkipped,
		progress.StateMounted:      copyPromptMounted,
	}
	var err error
	ch.tracked, err = track.NewTarget(gt, prompt, ch.tty)
	if err != nil {
		return nil, err
	}
	return ch.tracked, err
}

// StopTracking ends the copy tracking for the target.
func (ch *TTYCopyHandler) StopTracking() error {
	return ch.tracked.Close()
}

// OnCopySkipped is called when an object already exists.
func (ch *TTYCopyHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ch.tracked.Report(desc, progress.StateExists)
}

// PreCopy implements PreCopy of CopyHandler.
func (ch *TTYCopyHandler) PreCopy(context.Context, ocispec.Descriptor) error {
	return nil
}

// PostCopy implements PostCopy of CopyHandler.
func (ch *TTYCopyHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	successors, err := graph.FilteredSuccessors(ctx, desc, ch.tracked, DeduplicatedFilter(&ch.committed))
	if err != nil {
		return err
	}
	for _, successor := range successors {
		if err = ch.tracked.Report(successor, progress.StateSkipped); err != nil {
			return err
		}
	}
	return nil
}

// OnMounted implements OnMounted of CopyHandler.
func (ch *TTYCopyHandler) OnMounted(_ context.Context, desc ocispec.Descriptor) error {
	ch.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return ch.tracked.Report(desc, progress.StateMounted)
}

// TTYBackupHandler handles tty status output for backup events.
type TTYBackupHandler struct {
	tty       *os.File
	committed *sync.Map
	tracked   track.GraphTarget
	fetcher   content.Fetcher
}

// NewTTYBackupHandler returns a new handler for backup command.
func NewTTYBackupHandler(tty *os.File, fetcher content.Fetcher) BackupHandler {
	return &TTYBackupHandler{
		tty:       tty,
		committed: &sync.Map{},
		fetcher:   fetcher,
	}
}

// StartTracking returns a tracked target from a graph target.
func (bh *TTYBackupHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	prompts := map[progress.State]string{
		progress.StateInitialized:  backupPromptPulling,
		progress.StateTransmitting: backupPromptPulling,
		progress.StateTransmitted:  backupPromptPulled,
		progress.StateExists:       backupPromptExists,
		progress.StateSkipped:      backupPromptSkipped,
	}

	var err error
	bh.tracked, err = track.NewTarget(gt, prompts, bh.tty)
	if err != nil {
		return nil, err
	}
	return bh.tracked, err
}

// StopTracking ends the backup tracking for the target.
func (bh *TTYBackupHandler) StopTracking() error {
	return bh.tracked.Close()
}

// OnCopySkipped implements OnCopySkipped of BackupHandler.
func (bh *TTYBackupHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	bh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return bh.tracked.Report(desc, progress.StateExists)
}

// PreCopy implements PreCopy of BackupHandler.
func (bh *TTYBackupHandler) PreCopy(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// PostCopy implements PostCopy of BackupHandler.
func (bh *TTYBackupHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	bh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	successors, err := graph.FilteredSuccessors(ctx, desc, bh.fetcher, DeduplicatedFilter(bh.committed))
	if err != nil {
		return err
	}
	for _, successor := range successors {
		if err = bh.tracked.Report(successor, progress.StateSkipped); err != nil {
			return err
		}
	}
	return nil
}

// TTYRestoreHandler handles tty status output for restore events.
type TTYRestoreHandler struct {
	tty       *os.File
	committed *sync.Map
	tracked   track.GraphTarget
	fetcher   content.Fetcher
}

// NewTTYRestoreHandler returns a new handler for restore command.
func NewTTYRestoreHandler(tty *os.File, fetcher content.Fetcher) RestoreHandler {
	return &TTYRestoreHandler{
		tty:       tty,
		committed: &sync.Map{},
		fetcher:   fetcher,
	}
}

// StartTracking returns a tracked target from a graph target.
func (rh *TTYRestoreHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	prompts := map[progress.State]string{
		progress.StateInitialized:  restorePromptPushing,
		progress.StateTransmitting: restorePromptPushing,
		progress.StateTransmitted:  restorePromptPushed,
		progress.StateExists:       restorePromptExists,
		progress.StateSkipped:      restorePromptSkipped,
	}

	var err error
	rh.tracked, err = track.NewTarget(gt, prompts, rh.tty)
	if err != nil {
		return nil, err
	}
	return rh.tracked, err
}

// StopTracking ends the restore tracking for the target.
func (rh *TTYRestoreHandler) StopTracking() error {
	return rh.tracked.Close()
}

// OnCopySkipped implements OnCopySkipped of RestoreHandler.
func (rh *TTYRestoreHandler) OnCopySkipped(_ context.Context, desc ocispec.Descriptor) error {
	rh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	return rh.tracked.Report(desc, progress.StateExists)
}

// PreCopy implements PreCopy of RestoreHandler.
func (rh *TTYRestoreHandler) PreCopy(_ context.Context, _ ocispec.Descriptor) error {
	return nil
}

// PostCopy implements PostCopy of RestoreHandler.
func (rh *TTYRestoreHandler) PostCopy(ctx context.Context, desc ocispec.Descriptor) error {
	rh.committed.Store(desc.Digest.String(), desc.Annotations[ocispec.AnnotationTitle])
	successors, err := graph.FilteredSuccessors(ctx, desc, rh.fetcher, DeduplicatedFilter(rh.committed))
	if err != nil {
		return err
	}
	for _, successor := range successors {
		if err = rh.tracked.Report(successor, progress.StateSkipped); err != nil {
			return err
		}
	}
	return nil
}

// TTYBlobPushHandler handles tty status output for blob push events.
type TTYBlobPushHandler struct {
	desc    ocispec.Descriptor
	tty     *os.File
	tracked track.GraphTarget
}

// NewTTYBlobPushHandler returns a new handler for blob push command.
func NewTTYBlobPushHandler(tty *os.File, desc ocispec.Descriptor) BlobPushHandler {
	return &TTYBlobPushHandler{
		tty:  tty,
		desc: desc,
	}
}

// StartTracking returns a tracked target from a graph target.
func (bph *TTYBlobPushHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	prompt := map[progress.State]string{
		progress.StateInitialized:  PushPromptUploading,
		progress.StateTransmitting: PushPromptUploading,
		progress.StateTransmitted:  PushPromptUploaded,
		progress.StateExists:       PushPromptExists,
	}
	tracked, err := track.NewTarget(gt, prompt, bph.tty)
	if err != nil {
		return nil, err
	}
	bph.tracked = tracked
	return bph.tracked, nil
}

// StopTracking ends the blob push tracking for the target.
func (bph *TTYBlobPushHandler) StopTracking() error {
	return bph.tracked.Close()
}

// OnBlobExists implements BlobPushHandler.
func (bph *TTYBlobPushHandler) OnBlobExists() error {
	return bph.tracked.Report(bph.desc, progress.StateExists)
}

// OnBlobUploading implements BlobPushHandler.
func (bph *TTYBlobPushHandler) OnBlobUploading() error {
	return nil
}

// OnBlobUploaded implements BlobPushHandler.
func (bph *TTYBlobPushHandler) OnBlobUploaded() error {
	return nil
}
