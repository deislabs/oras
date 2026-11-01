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
	"oras.land/oras-go/v2"
	"oras.land/oras/cmd/oras/internal/display/status"
)

// RecordingPullStatusHandler records pull status handler calls.
type RecordingPullStatusHandler struct {
	status.DiscardHandler
	Recorder           *CallRecorder
	TrackErr           error
	NodeProcessingErr  error
	NodeDownloadingErr error
}

// TrackTarget records target tracking and returns the configured error.
func (h *RecordingPullStatusHandler) TrackTarget(gt oras.GraphTarget) (oras.GraphTarget, status.StopTrackTargetFunc, error) {
	h.Recorder.Record("TrackTarget")
	return gt, func() error {
		h.Recorder.Record("StopTrackTarget")
		return nil
	}, h.TrackErr
}

// OnNodeProcessing records manifest processing and returns the configured error.
func (h *RecordingPullStatusHandler) OnNodeProcessing(ocispec.Descriptor) error {
	h.Recorder.Record("OnNodeProcessing")
	return h.NodeProcessingErr
}

// OnNodeDownloading records node downloading and returns the configured error.
func (h *RecordingPullStatusHandler) OnNodeDownloading(ocispec.Descriptor) error {
	h.Recorder.Record("OnNodeDownloading")
	return h.NodeDownloadingErr
}

// OnNodeDownloaded records node completion.
func (h *RecordingPullStatusHandler) OnNodeDownloaded(ocispec.Descriptor) error {
	h.Recorder.Record("OnNodeDownloaded")
	return nil
}

// OnNodeRestored records node restoration.
func (h *RecordingPullStatusHandler) OnNodeRestored(ocispec.Descriptor) error {
	h.Recorder.Record("OnNodeRestored")
	return nil
}

// OnNodeSkipped records node skipping.
func (h *RecordingPullStatusHandler) OnNodeSkipped(ocispec.Descriptor) error {
	h.Recorder.Record("OnNodeSkipped")
	return nil
}
