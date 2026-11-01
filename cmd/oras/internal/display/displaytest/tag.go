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
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/display/metadata"
)

// RecordingTagHandler records tag metadata handler calls.
type RecordingTagHandler struct {
	metadata.Discard
	Recorder   *CallRecorder
	TaggingErr error
}

func (h *RecordingTagHandler) OnTagging(_ ocispec.Descriptor, _ string) error {
	h.Recorder.Record("OnTagging")
	return h.TaggingErr
}

func (h *RecordingTagHandler) OnTagged(_ ocispec.Descriptor, _ string) error {
	h.Recorder.Record("OnTagged")
	return nil
}

var _ metadata.TagHandler = (*RecordingTagHandler)(nil)
