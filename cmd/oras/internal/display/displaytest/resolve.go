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

// RecordingResolveHandler records resolve metadata handler calls.
type RecordingResolveHandler struct {
	metadata.Discard
	Recorder *CallRecorder
	Err      error
}

func (h *RecordingResolveHandler) OnResolved(_ ocispec.Descriptor) error {
	h.Recorder.Record("OnResolved")
	return h.Err
}

var _ metadata.ResolveHandler = (*RecordingResolveHandler)(nil)
