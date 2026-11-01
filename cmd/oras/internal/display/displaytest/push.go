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
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/option"
)

// CallRecorder records handler calls in the order they occur.
type CallRecorder struct {
	mu    sync.Mutex
	Calls []string
}

// Record appends a call to the log.
func (r *CallRecorder) Record(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Calls = append(r.Calls, call)
}

// Snapshot returns a copy of the recorded calls.
func (r *CallRecorder) Snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.Calls...)
}

// RecordingPushStatusHandler records push status handler calls.
type RecordingPushStatusHandler struct {
	status.DiscardHandler
	Recorder        *CallRecorder
	FileLoadingErr  error
	ManifestPushErr error
}

// OnFileLoading records the file-loading event and returns the configured error.
func (h *RecordingPushStatusHandler) OnFileLoading(_ string) error {
	h.Recorder.Record("OnFileLoading")
	return h.FileLoadingErr
}

func (h *RecordingPushStatusHandler) OnManifestPushSkipped() error {
	h.Recorder.Record("OnManifestPushSkipped")
	return h.ManifestPushErr
}

func (h *RecordingPushStatusHandler) OnManifestPushing() error {
	h.Recorder.Record("OnManifestPushing")
	return h.ManifestPushErr
}

func (h *RecordingPushStatusHandler) OnManifestPushed() error {
	h.Recorder.Record("OnManifestPushed")
	return h.ManifestPushErr
}

// RecordingPushMetadataHandler records push metadata handler calls.
type RecordingPushMetadataHandler struct {
	metadata.Discard
	Recorder          *CallRecorder
	CopiedErr         error
	TaggedErr         error
	RenderErr         error
	ManifestPushedErr error
}

// OnCopied records the copied event and returns the configured error.
func (h *RecordingPushMetadataHandler) OnCopied(
	_ *option.Target,
	_ ocispec.Descriptor,
) error {
	h.Recorder.Record("OnCopied")
	return h.CopiedErr
}

// OnTagged records the tag event and returns the configured error.
func (h *RecordingPushMetadataHandler) OnTagged(
	_ ocispec.Descriptor,
	tag string,
) error {
	h.Recorder.Record("OnTagged:" + tag)
	return h.TaggedErr
}

func (h *RecordingPushMetadataHandler) OnManifestPushed(_ ocispec.Descriptor) error {
	h.Recorder.Record("OnManifestPushed")
	return h.ManifestPushedErr
}

// Render records the render call and returns the configured error.
func (h *RecordingPushMetadataHandler) Render() error {
	h.Recorder.Record("Render")
	return h.RenderErr
}
