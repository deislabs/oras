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

package status

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/cmd/oras/internal/output"
	"oras.land/oras/internal/testutils"
)

var (
	ctx         context.Context
	builder     *strings.Builder
	printer     *output.Printer
	bogus       ocispec.Descriptor
	mockFetcher testutils.MockFetcher
)

func TestMain(m *testing.M) {
	mockFetcher = testutils.NewMockFetcher()
	ctx = context.Background()
	builder = &strings.Builder{}
	printer = output.NewPrinter(builder, os.Stderr)
	bogus = ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest}
	os.Exit(m.Run())
}

func validatePrinted(t *testing.T, expected string) {
	t.Helper()
	actual := strings.TrimSpace(builder.String())
	if expected != actual {
		t.Error("Output does not match expected <" + expected + "> actual <" + actual + ">")
	}
}

func TestTextCopyHandler_OnMounted(t *testing.T) {
	builder.Reset()
	ch := NewTextCopyHandler(printer, mockFetcher.Fetcher)
	if ch.OnMounted(ctx, mockFetcher.OciImage) != nil {
		t.Error("OnMounted() should not return an error")
	}
	validatePrinted(t, "Mounted 0b442c23c1dd oci-image")
}

func TestTextCopyHandler_OnCopySkipped(t *testing.T) {
	builder.Reset()
	ch := NewTextCopyHandler(printer, mockFetcher.Fetcher)
	if ch.OnCopySkipped(ctx, mockFetcher.OciImage) != nil {
		t.Error("OnCopySkipped() should not return an error")
	}
	validatePrinted(t, "Exists  0b442c23c1dd oci-image")
}

func TestTextCopyHandler_PostCopy_titled(t *testing.T) {
	builder.Reset()
	ch := NewTextCopyHandler(printer, mockFetcher.Fetcher)
	if ch.PostCopy(ctx, mockFetcher.OciImage) != nil {
		t.Error("PostCopy() should not return an error")
	}
	if ch.PostCopy(ctx, bogus) == nil {
		t.Error("PostCopy() should return an error")
	}
	validatePrinted(t, "Copied  0b442c23c1dd oci-image")
}

func TestTextCopyHandler_PostCopy_skipped(t *testing.T) {
	builder.Reset()
	ch := &TextCopyHandler{
		printer:   printer,
		fetcher:   mockFetcher.Fetcher,
		committed: &sync.Map{},
	}
	ch.committed.Store(mockFetcher.ImageLayer.Digest.String(), mockFetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"bogus")
	if err := ch.PostCopy(ctx, mockFetcher.OciImage); err != nil {
		t.Error("PostCopy() returns unexpected err:", err)
	}
	validatePrinted(t, "Skipped f6b87e8e0fe1 layer\nCopied  0b442c23c1dd oci-image")
}

func TestTextCopyHandler_PreCopy(t *testing.T) {
	builder.Reset()
	ch := NewTextCopyHandler(printer, mockFetcher.Fetcher)
	if ch.PreCopy(ctx, mockFetcher.OciImage) != nil {
		t.Error("PreCopy() should not return an error")
	}
	validatePrinted(t, "Copying 0b442c23c1dd oci-image")
}

func TestTextPullHandler_OnNodeDownloaded(t *testing.T) {
	builder.Reset()
	ph := NewTextPullHandler(printer)
	if ph.OnNodeDownloaded(mockFetcher.OciImage) != nil {
		t.Error("OnNodeDownloaded() should not return an error")
	}
	validatePrinted(t, "Downloaded  0b442c23c1dd oci-image")
}

func TestTextPullHandler_OnNodeDownloading(t *testing.T) {
	builder.Reset()
	ph := NewTextPullHandler(printer)
	if ph.OnNodeDownloading(mockFetcher.OciImage) != nil {
		t.Error("OnNodeDownloading() should not return an error")
	}
	validatePrinted(t, "Downloading 0b442c23c1dd oci-image")
}

func TestTextPullHandler_OnNodeProcessing(t *testing.T) {
	builder.Reset()
	ph := NewTextPullHandler(printer)
	if ph.OnNodeProcessing(mockFetcher.OciImage) != nil {
		t.Error("OnNodeProcessing() should not return an error")
	}
	validatePrinted(t, "Processing  0b442c23c1dd oci-image")
}

func TestTextPullHandler_OnNodeRestored(t *testing.T) {
	builder.Reset()
	ph := NewTextPullHandler(printer)
	if ph.OnNodeRestored(mockFetcher.OciImage) != nil {
		t.Error("OnNodeRestored() should not return an error")
	}
	validatePrinted(t, "Restored    0b442c23c1dd oci-image")
}

func TestTextPullHandler_OnNodeSkipped(t *testing.T) {
	builder.Reset()
	ph := NewTextPullHandler(printer)
	if ph.OnNodeSkipped(mockFetcher.OciImage) != nil {
		t.Error("OnNodeSkipped() should not return an error")
	}
	validatePrinted(t, "Skipped     0b442c23c1dd oci-image")
}

func TestTextPushHandler_OnCopySkipped(t *testing.T) {
	builder.Reset()
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	if ph.OnCopySkipped(ctx, mockFetcher.OciImage) != nil {
		t.Error("OnCopySkipped() should not return an error")
	}
	validatePrinted(t, "Exists    0b442c23c1dd oci-image")
}

func TestTextPushHandler_OnEmptyArtifact(t *testing.T) {
	builder.Reset()
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	if ph.OnEmptyArtifact() != nil {
		t.Error("OnEmptyArtifact() should not return an error")
	}
	validatePrinted(t, "Uploading empty artifact")
}

func TestTextPushHandler_OnFileLoading(t *testing.T) {
	builder.Reset()
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	if ph.OnFileLoading("name") != nil {
		t.Error("OnFileLoading() should not return an error")
	}
	validatePrinted(t, "")
}

func TestTextPushHandler_PostCopy(t *testing.T) {
	builder.Reset()
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	if ph.PostCopy(ctx, mockFetcher.OciImage) != nil {
		t.Error("PostCopy() should not return an error")
	}
	validatePrinted(t, "Uploaded  0b442c23c1dd oci-image")
}

func TestTextPushHandler_PreCopy(t *testing.T) {
	builder.Reset()
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	if ph.PreCopy(ctx, mockFetcher.OciImage) != nil {
		t.Error("PreCopy() should not return an error")
	}
	validatePrinted(t, "Uploading 0b442c23c1dd oci-image")
}

func TestTextManifestPushHandler_OnPushSkipped(t *testing.T) {
	mph := NewTextManifestPushHandler(printer, ocispec.Descriptor{})
	if mph.OnManifestPushSkipped() != nil {
		t.Error("OnManifestExists() should not return an error")
	}
}

func TestTextManifestIndexUpdateHandler_OnManifestAdded(t *testing.T) {
	tests := []struct {
		name    string
		printer *output.Printer
		ref     string
		desc    ocispec.Descriptor
		wantErr bool
	}{
		{
			name:    "ref is a digest",
			printer: output.NewPrinter(os.Stdout, os.Stderr),
			ref:     "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb",
			desc:    ocispec.Descriptor{MediaType: "test", Digest: "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb", Size: 25},
			wantErr: false,
		},
		{
			name:    "ref is not a digest",
			printer: output.NewPrinter(os.Stdout, os.Stderr),
			ref:     "v1",
			desc:    ocispec.Descriptor{MediaType: "test", Digest: "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb", Size: 25},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			miuh := &TextManifestIndexUpdateHandler{
				printer: tt.printer,
			}
			if err := miuh.OnManifestAdded(tt.ref, tt.desc); (err != nil) != tt.wantErr {
				t.Errorf("TextManifestIndexUpdateHandler.OnManifestAdded() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTextManifestIndexUpdateHandler_OnIndexMerged(t *testing.T) {
	tests := []struct {
		name    string
		printer *output.Printer
		ref     string
		desc    ocispec.Descriptor
		wantErr bool
	}{
		{
			name:    "ref is a digest",
			printer: output.NewPrinter(os.Stdout, os.Stderr),
			ref:     "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb",
			desc:    ocispec.Descriptor{MediaType: "test", Digest: "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb", Size: 25},
			wantErr: false,
		},
		{
			name:    "ref is not a digest",
			printer: output.NewPrinter(os.Stdout, os.Stderr),
			ref:     "v1",
			desc:    ocispec.Descriptor{MediaType: "test", Digest: "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb", Size: 25},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			miuh := &TextManifestIndexUpdateHandler{
				printer: tt.printer,
			}
			if err := miuh.OnIndexMerged(tt.ref, tt.desc); (err != nil) != tt.wantErr {
				t.Errorf("TextManifestIndexUpdateHandler.OnIndexMerged() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTextBackupHandler(t *testing.T) {
	t.Run("OnCopySkipped", func(t *testing.T) {
		builder.Reset()
		bh := NewTextBackupHandler(printer, mockFetcher.Fetcher)
		if bh.OnCopySkipped(ctx, mockFetcher.OciImage) != nil {
			t.Error("OnCopySkipped() should not return an error")
		}
		validatePrinted(t, "Exists    0b442c23c1dd oci-image")
	})

	t.Run("PreCopy", func(t *testing.T) {
		builder.Reset()
		bh := NewTextBackupHandler(printer, mockFetcher.Fetcher)
		if bh.PreCopy(ctx, mockFetcher.OciImage) != nil {
			t.Error("PreCopy() should not return an error")
		}
		validatePrinted(t, "Pulling   0b442c23c1dd oci-image")
	})

	t.Run("PostCopy", func(t *testing.T) {
		builder.Reset()
		bh := NewTextBackupHandler(printer, mockFetcher.Fetcher)
		if bh.PostCopy(ctx, mockFetcher.OciImage) != nil {
			t.Error("PostCopy() should not return an error")
		}
		validatePrinted(t, "Pulled    0b442c23c1dd oci-image")
	})

	t.Run("PostCopy_Skipped", func(t *testing.T) {
		builder.Reset()
		bh := &TextBackupHandler{
			printer:   printer,
			fetcher:   mockFetcher.Fetcher,
			committed: &sync.Map{},
		}
		bh.committed.Store(mockFetcher.ImageLayer.Digest.String(), mockFetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"bogus")
		if err := bh.PostCopy(ctx, mockFetcher.OciImage); err != nil {
			t.Error("PostCopy() returns unexpected err:", err)
		}
		validatePrinted(t, "Skipped   f6b87e8e0fe1 layer\nPulled    0b442c23c1dd oci-image")
	})

	t.Run("StartTracking", func(t *testing.T) {
		bh := NewTextBackupHandler(printer, mockFetcher.Fetcher)
		gt := memory.New()
		result, err := bh.StartTracking(gt)
		if err != nil {
			t.Error("StartTracking() should not return an error")
		}
		if result != gt {
			t.Error("StartTracking() should return the same GraphTarget")
		}
	})

	t.Run("StopTracking", func(t *testing.T) {
		bh := NewTextBackupHandler(printer, mockFetcher.Fetcher)
		if bh.StopTracking() != nil {
			t.Error("StopTracking() should not return an error")
		}
	})
}

func TestTextRestoreHandler(t *testing.T) {
	t.Run("OnCopySkipped", func(t *testing.T) {
		builder.Reset()
		rh := NewTextRestoreHandler(printer, mockFetcher.Fetcher)
		if rh.OnCopySkipped(ctx, mockFetcher.OciImage) != nil {
			t.Error("OnCopySkipped() should not return an error")
		}
		validatePrinted(t, "Exists    0b442c23c1dd oci-image")
	})

	t.Run("PreCopy", func(t *testing.T) {
		builder.Reset()
		rh := NewTextRestoreHandler(printer, mockFetcher.Fetcher)
		if rh.PreCopy(ctx, mockFetcher.OciImage) != nil {
			t.Error("PreCopy() should not return an error")
		}
		validatePrinted(t, "Pushing   0b442c23c1dd oci-image")
	})

	t.Run("PostCopy", func(t *testing.T) {
		builder.Reset()
		rh := NewTextRestoreHandler(printer, mockFetcher.Fetcher)
		if rh.PostCopy(ctx, mockFetcher.OciImage) != nil {
			t.Error("PostCopy() should not return an error")
		}
		validatePrinted(t, "Pushed    0b442c23c1dd oci-image")
	})

	t.Run("PostCopy_Skipped", func(t *testing.T) {
		builder.Reset()
		rh := &TextRestoreHandler{
			printer:   printer,
			fetcher:   mockFetcher.Fetcher,
			committed: &sync.Map{},
		}
		rh.committed.Store(mockFetcher.ImageLayer.Digest.String(), mockFetcher.ImageLayer.Annotations[ocispec.AnnotationTitle]+"bogus")
		if err := rh.PostCopy(ctx, mockFetcher.OciImage); err != nil {
			t.Error("PostCopy() returns unexpected err:", err)
		}
		validatePrinted(t, "Skipped   f6b87e8e0fe1 layer\nPushed    0b442c23c1dd oci-image")
	})

	t.Run("StartTracking", func(t *testing.T) {
		rh := NewTextRestoreHandler(printer, mockFetcher.Fetcher)
		gt := memory.New()
		result, err := rh.StartTracking(gt)
		if err != nil {
			t.Error("StartTracking() should not return an error")
		}
		if result != gt {
			t.Error("StartTracking() should return the same GraphTarget")
		}
	})

	t.Run("StopTracking", func(t *testing.T) {
		rh := NewTextRestoreHandler(printer, mockFetcher.Fetcher)
		if rh.StopTracking() != nil {
			t.Error("StopTracking() should not return an error")
		}
	})
}

func TestTextPushHandler_TrackTarget(t *testing.T) {
	ph := NewTextPushHandler(printer, mockFetcher.Fetcher)
	gt := memory.New()
	tracked, stop, err := ph.TrackTarget(gt)
	if err != nil {
		t.Fatalf("TrackTarget() error = %v", err)
	}
	if tracked != gt {
		t.Error("TrackTarget() should return the original target")
	}
	if err := stop(); err != nil {
		t.Errorf("stop() error = %v", err)
	}
}

func TestTextAttachHandler_PreCopy(t *testing.T) {
	builder.Reset()
	ah := NewTextAttachHandler(printer, mockFetcher.Fetcher)
	if err := ah.PreCopy(ctx, mockFetcher.OciImage); err != nil {
		t.Fatalf("PreCopy() error = %v", err)
	}
	validatePrinted(t, "Uploading 0b442c23c1dd oci-image")
}

func TestTextPullHandler_TrackTarget(t *testing.T) {
	ph := NewTextPullHandler(printer)
	gt := memory.New()
	tracked, stop, err := ph.TrackTarget(gt)
	if err != nil {
		t.Fatalf("TrackTarget() error = %v", err)
	}
	if tracked != gt {
		t.Error("TrackTarget() should return the original target")
	}
	if err := stop(); err != nil {
		t.Errorf("stop() error = %v", err)
	}
}

func TestTextCopyHandler_StartStopTracking(t *testing.T) {
	ch := NewTextCopyHandler(printer, mockFetcher.Fetcher)
	gt := memory.New()
	tracked, err := ch.StartTracking(gt)
	if err != nil {
		t.Fatalf("StartTracking() error = %v", err)
	}
	if tracked != gt {
		t.Error("StartTracking() should return the original target")
	}
	if err := ch.StopTracking(); err != nil {
		t.Errorf("StopTracking() error = %v", err)
	}
}

func TestTextManifestPushHandler(t *testing.T) {
	tests := []struct {
		name string
		call func(ManifestPushHandler) error
		want string
	}{
		{"OnManifestPushSkipped", ManifestPushHandler.OnManifestPushSkipped, "Exists    0b442c23c1dd oci-image"},
		{"OnManifestPushing", ManifestPushHandler.OnManifestPushing, "Uploading 0b442c23c1dd oci-image"},
		{"OnManifestPushed", ManifestPushHandler.OnManifestPushed, "Uploaded  0b442c23c1dd oci-image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder.Reset()
			if err := tt.call(NewTextManifestPushHandler(printer, mockFetcher.OciImage)); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			validatePrinted(t, tt.want)
		})
	}
}

func TestTextManifestIndexCreateHandler(t *testing.T) {
	desc := mockFetcher.OciImage
	digestRef := desc.Digest.String()
	tests := []struct {
		name string
		call func(ManifestIndexCreateHandler) error
		want string
	}{
		{"OnFetching", func(h ManifestIndexCreateHandler) error { return h.OnFetching("v1") }, "Fetching  v1"},
		{"OnFetched tag", func(h ManifestIndexCreateHandler) error { return h.OnFetched("v1", desc) }, "Fetched   " + digestRef + " v1"},
		{"OnFetched digest", func(h ManifestIndexCreateHandler) error { return h.OnFetched(digestRef, desc) }, "Fetched   " + digestRef},
		{"OnIndexPacked", func(h ManifestIndexCreateHandler) error { return h.OnIndexPacked(desc) }, "Packed    0b442c23c1dd " + ocispec.MediaTypeImageIndex},
		{"OnIndexPushed", func(h ManifestIndexCreateHandler) error { return h.OnIndexPushed("localhost:5000/repo:v1") }, "Pushed    localhost:5000/repo:v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder.Reset()
			if err := tt.call(NewTextManifestIndexCreateHandler(printer)); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			validatePrinted(t, tt.want)
		})
	}
}

func TestTextManifestIndexUpdateHandler(t *testing.T) {
	desc := mockFetcher.OciImage
	digestRef := desc.Digest.String()
	tests := []struct {
		name string
		call func(ManifestIndexUpdateHandler) error
		want string
	}{
		{"OnFetching", func(h ManifestIndexUpdateHandler) error { return h.OnFetching("v1") }, "Fetching  v1"},
		{"OnFetched tag", func(h ManifestIndexUpdateHandler) error { return h.OnFetched("v1", desc) }, "Fetched   " + digestRef + " v1"},
		{"OnFetched digest", func(h ManifestIndexUpdateHandler) error { return h.OnFetched(digestRef, desc) }, "Fetched   " + digestRef},
		{"OnManifestRemoved", func(h ManifestIndexUpdateHandler) error { return h.OnManifestRemoved(desc.Digest) }, "Removed   " + digestRef},
		{"OnManifestAdded tag", func(h ManifestIndexUpdateHandler) error { return h.OnManifestAdded("v1", desc) }, "Added     " + digestRef + " v1"},
		{"OnManifestAdded digest", func(h ManifestIndexUpdateHandler) error { return h.OnManifestAdded(digestRef, desc) }, "Added     " + digestRef},
		{"OnIndexMerged tag", func(h ManifestIndexUpdateHandler) error { return h.OnIndexMerged("v1", desc) }, "Merged    " + digestRef + " v1"},
		{"OnIndexMerged digest", func(h ManifestIndexUpdateHandler) error { return h.OnIndexMerged(digestRef, desc) }, "Merged    " + digestRef},
		{"OnIndexPacked", func(h ManifestIndexUpdateHandler) error { return h.OnIndexPacked(desc) }, "Updated   " + digestRef},
		{"OnIndexPushed", func(h ManifestIndexUpdateHandler) error { return h.OnIndexPushed("localhost:5000/repo:v1") }, "Pushed    localhost:5000/repo:v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder.Reset()
			if err := tt.call(NewTextManifestIndexUpdateHandler(printer)); err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			validatePrinted(t, tt.want)
		})
	}
}

func TestTextBlobPushHandler(t *testing.T) {
	tests := []struct {
		name string
		call func(BlobPushHandler) error
		want string
	}{
		{"OnBlobExists", BlobPushHandler.OnBlobExists, "Exists    0b442c23c1dd oci-image"},
		{"OnBlobUploading", BlobPushHandler.OnBlobUploading, "Uploading 0b442c23c1dd oci-image"},
		{"OnBlobUploaded", BlobPushHandler.OnBlobUploaded, "Uploaded  0b442c23c1dd oci-image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder.Reset()
			if err := tt.call(NewTextBlobPushHandler(printer, mockFetcher.OciImage)); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			validatePrinted(t, tt.want)
		})
	}
}

func TestTextBlobPushHandler_StartStopTracking(t *testing.T) {
	bh := NewTextBlobPushHandler(printer, mockFetcher.OciImage)
	gt := memory.New()
	tracked, err := bh.StartTracking(gt)
	if err != nil {
		t.Fatalf("StartTracking() error = %v", err)
	}
	if tracked != gt {
		t.Error("StartTracking() should return the original target")
	}
	if err := bh.StopTracking(); err != nil {
		t.Errorf("StopTracking() error = %v", err)
	}
}
