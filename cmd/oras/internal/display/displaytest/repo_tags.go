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

// RecordingRepoTagsHandler records repo-tags metadata handler calls.
type RecordingRepoTagsHandler struct {
	metadata.Discard
	Recorder     *CallRecorder
	TagListedErr error
}

func (h *RecordingRepoTagsHandler) OnTagListed(string) error {
	h.Recorder.Record("OnTagListed")
	return h.TagListedErr
}

func (h *RecordingRepoTagsHandler) Render() error {
	h.Recorder.Record("Render")
	return nil
}

var _ metadata.RepoTagsHandler = (*RecordingRepoTagsHandler)(nil)
