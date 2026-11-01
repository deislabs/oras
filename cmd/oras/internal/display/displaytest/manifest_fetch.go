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

	"oras.land/oras/cmd/oras/internal/display/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
)

// RecordingManifestFetchContentHandler records manifest fetch content handler calls.
type RecordingManifestFetchContentHandler struct {
	Recorder          *CallRecorder
	ContentFetchedErr error
}

// OnContentFetched records the fetched content event and returns the configured error.
func (h *RecordingManifestFetchContentHandler) OnContentFetched(ocispec.Descriptor, []byte) error {
	h.Recorder.Record("OnContentFetched")
	return h.ContentFetchedErr
}

// RecordingManifestFetchMetadataHandler records manifest fetch metadata handler calls.
type RecordingManifestFetchMetadataHandler struct {
	Recorder   *CallRecorder
	FetchedErr error
}

// OnFetched records the fetched manifest event and returns the configured error.
func (h *RecordingManifestFetchMetadataHandler) OnFetched(string, ocispec.Descriptor, []byte) error {
	h.Recorder.Record("OnFetched")
	return h.FetchedErr
}

var _ content.ManifestFetchHandler = (*RecordingManifestFetchContentHandler)(nil)
var _ metadata.ManifestFetchHandler = (*RecordingManifestFetchMetadataHandler)(nil)
