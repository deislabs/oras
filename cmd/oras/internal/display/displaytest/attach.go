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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/option"
)

// RecordingAttachStatusHandler records attach status handler calls.
type RecordingAttachStatusHandler struct {
	status.DiscardHandler
	Recorder *CallRecorder
	TrackErr error
}

func (h *RecordingAttachStatusHandler) OnFileLoading(string) error {
	h.Recorder.Record("OnFileLoading")
	return nil
}

func (h *RecordingAttachStatusHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, status.StopTrackTargetFunc, error) {
	h.Recorder.Record("TrackTarget")
	return gt, func() error {
		h.Recorder.Record("StopTrackTarget")
		return nil
	}, h.TrackErr
}

func (h *RecordingAttachStatusHandler) OnCopySkipped(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("OnCopySkipped")
	return nil
}

func (h *RecordingAttachStatusHandler) PreCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PreCopy")
	return nil
}

func (h *RecordingAttachStatusHandler) PostCopy(context.Context, ocispec.Descriptor) error {
	h.Recorder.Record("PostCopy")
	return nil
}

// RecordingAttachMetadataHandler records attach metadata handler calls.
type RecordingAttachMetadataHandler struct {
	metadata.Discard
	Recorder  *CallRecorder
	RenderErr error
}

func (h *RecordingAttachMetadataHandler) OnAttached(*option.Target, ocispec.Descriptor, ocispec.Descriptor) {
	h.Recorder.Record("OnAttached")
}

func (h *RecordingAttachMetadataHandler) Render() error {
	h.Recorder.Record("Render")
	return h.RenderErr
}

var _ status.AttachHandler = (*RecordingAttachStatusHandler)(nil)
var _ metadata.AttachHandler = (*RecordingAttachMetadataHandler)(nil)
