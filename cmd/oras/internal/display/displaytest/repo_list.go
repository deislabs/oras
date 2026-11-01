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

// RecordingRepoListHandler records repo-list metadata handler calls.
type RecordingRepoListHandler struct {
	metadata.Discard
	Recorder            *CallRecorder
	RepositoryListedErr error
}

// OnRepositoryListed records the listed repository and returns the configured error.
func (h *RecordingRepoListHandler) OnRepositoryListed(repo string) error {
	h.Recorder.Record("OnRepositoryListed:" + repo)
	return h.RepositoryListedErr
}

// Render records the render call.
func (h *RecordingRepoListHandler) Render() error {
	h.Recorder.Record("Render")
	return nil
}

var _ metadata.RepoListHandler = (*RecordingRepoListHandler)(nil)
