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

import "oras.land/oras/cmd/oras/internal/display/metadata"

// RecordingManifestDeleteHandler records manifest-delete metadata handler calls.
type RecordingManifestDeleteHandler struct {
	metadata.Discard
	Recorder   *CallRecorder
	MissingErr error
}

// OnManifestMissing records the missing event and returns the configured error.
func (h *RecordingManifestDeleteHandler) OnManifestMissing() error {
	h.Recorder.Record("OnManifestMissing")
	return h.MissingErr
}

// OnManifestDeleted records the deleted event.
func (h *RecordingManifestDeleteHandler) OnManifestDeleted() error {
	h.Recorder.Record("OnManifestDeleted")
	return nil
}

var _ metadata.ManifestDeleteHandler = (*RecordingManifestDeleteHandler)(nil)
