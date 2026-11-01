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

package displaytest

import (
	"context"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
)

// RecordingBackupStatusHandler records backup status handler calls.
type RecordingBackupStatusHandler struct {
	status.DiscardHandler
	Recorder   *CallRecorder
	PreCopyErr error
}

func (h *RecordingBackupStatusHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	h.Recorder.Record("StartTracking")
	return gt, nil
}

func (h *RecordingBackupStatusHandler) StopTracking() error {
	h.Recorder.Record("StopTracking")
	return nil
}

func (h *RecordingBackupStatusHandler) OnCopySkipped(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("OnCopySkipped")
	return nil
}

func (h *RecordingBackupStatusHandler) PreCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PreCopy")
	return h.PreCopyErr
}

func (h *RecordingBackupStatusHandler) PostCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PostCopy")
	return nil
}

// RecordingBackupMetadataHandler records backup metadata handler calls.
type RecordingBackupMetadataHandler struct {
	metadata.Discard
	Recorder        *CallRecorder
	TagsFoundErr    error
	TarExportingErr error
	TarExportedErr  error
	CompletedErr    error
}

func (h *RecordingBackupMetadataHandler) OnTagsFound([]string) error {
	h.Recorder.Record("OnTagsFound")
	return h.TagsFoundErr
}

func (h *RecordingBackupMetadataHandler) OnArtifactPulled(string, int) error {
	h.Recorder.Record("OnArtifactPulled")
	return nil
}

func (h *RecordingBackupMetadataHandler) OnTarExporting(string) error {
	h.Recorder.Record("OnTarExporting")
	return h.TarExportingErr
}

func (h *RecordingBackupMetadataHandler) OnTarExported(string, int64) error {
	h.Recorder.Record("OnTarExported")
	return h.TarExportedErr
}

func (h *RecordingBackupMetadataHandler) OnBackupCompleted(int, string, time.Duration) error {
	h.Recorder.Record("OnBackupCompleted")
	return h.CompletedErr
}

var _ status.BackupHandler = (*RecordingBackupStatusHandler)(nil)
var _ metadata.BackupHandler = (*RecordingBackupMetadataHandler)(nil)
