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
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/option"
)

// RecordingCopyStatusHandler records copy status handler calls.
type RecordingCopyStatusHandler struct {
	status.DiscardHandler
	Recorder *CallRecorder
}

// OnCopySkipped records the skipped-copy event.
func (h *RecordingCopyStatusHandler) OnCopySkipped(_ context.Context, _ ocispec.Descriptor) error {
	h.Recorder.Record("OnCopySkipped")
	return nil
}

// PreCopy records the start of a copy.
func (h *RecordingCopyStatusHandler) PreCopy(_ context.Context, _ ocispec.Descriptor) error {
	h.Recorder.Record("PreCopy")
	return nil
}

// PostCopy records the completion of a copy.
func (h *RecordingCopyStatusHandler) PostCopy(_ context.Context, _ ocispec.Descriptor) error {
	h.Recorder.Record("PostCopy")
	return nil
}

// OnMounted records a successful mount.
func (h *RecordingCopyStatusHandler) OnMounted(_ context.Context, _ ocispec.Descriptor) error {
	h.Recorder.Record("OnMounted")
	return nil
}

// RecordingCopyMetadataHandler records copy metadata handler calls.
type RecordingCopyMetadataHandler struct {
	metadata.Discard
	Recorder  *CallRecorder
	CopiedErr error
	RenderErr error
}

// OnCopied records the copied event and returns the configured error.
func (h *RecordingCopyMetadataHandler) OnCopied(_ *option.BinaryTarget, _ ocispec.Descriptor) error {
	h.Recorder.Record("OnCopied")
	return h.CopiedErr
}

// Render records the render call and returns the configured error.
func (h *RecordingCopyMetadataHandler) Render() error {
	h.Recorder.Record("Render")
	return h.RenderErr
}
