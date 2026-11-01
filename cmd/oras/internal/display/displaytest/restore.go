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

// RecordingRestoreStatusHandler records restore status handler calls.
type RecordingRestoreStatusHandler struct {
	status.DiscardHandler
	Recorder   *CallRecorder
	PreCopyErr error
}

func (h *RecordingRestoreStatusHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	h.Recorder.Record("StartTracking")
	return gt, nil
}

func (h *RecordingRestoreStatusHandler) StopTracking() error {
	h.Recorder.Record("StopTracking")
	return nil
}

func (h *RecordingRestoreStatusHandler) OnCopySkipped(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("OnCopySkipped")
	return nil
}

func (h *RecordingRestoreStatusHandler) PreCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PreCopy")
	return h.PreCopyErr
}

func (h *RecordingRestoreStatusHandler) PostCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PostCopy")
	return nil
}

// RecordingRestoreMetadataHandler records restore metadata handler calls.
type RecordingRestoreMetadataHandler struct {
	metadata.Discard
	Recorder          *CallRecorder
	TagsFoundErr      error
	ArtifactPushedErr error
	CompletedErr      error
}

func (h *RecordingRestoreMetadataHandler) OnTarLoaded(string, int64) error {
	h.Recorder.Record("OnTarLoaded")
	return nil
}

func (h *RecordingRestoreMetadataHandler) OnTagsFound([]string) error {
	h.Recorder.Record("OnTagsFound")
	return h.TagsFoundErr
}

func (h *RecordingRestoreMetadataHandler) OnArtifactPushed(string, int) error {
	h.Recorder.Record("OnArtifactPushed")
	return h.ArtifactPushedErr
}

func (h *RecordingRestoreMetadataHandler) OnRestoreCompleted(int, string, time.Duration) error {
	h.Recorder.Record("OnRestoreCompleted")
	return h.CompletedErr
}

var _ status.RestoreHandler = (*RecordingRestoreStatusHandler)(nil)
var _ metadata.RestoreHandler = (*RecordingRestoreMetadataHandler)(nil)
