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

// RecordingBlobDeleteHandler records blob-delete metadata handler calls.
type RecordingBlobDeleteHandler struct {
	metadata.Discard
	Recorder   *CallRecorder
	MissingErr error
}

func (h *RecordingBlobDeleteHandler) OnBlobMissing() error {
	h.Recorder.Record("OnBlobMissing")
	return h.MissingErr
}

func (h *RecordingBlobDeleteHandler) OnBlobDeleted() error {
	h.Recorder.Record("OnBlobDeleted")
	return nil
}

var _ metadata.BlobDeleteHandler = (*RecordingBlobDeleteHandler)(nil)
