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

package text

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

func TestNewPullHandler(t *testing.T) {
	printer := output.NewPrinter(&bytes.Buffer{}, os.Stderr)

	handler := NewPullHandler(printer)
	if handler == nil {
		t.Fatal("expected a non-nil handler")
	}

	ph, ok := handler.(*PullHandler)
	if !ok {
		t.Fatal("expected handler to be of type *PullHandler")
	}
	if ph.printer != printer {
		t.Error("handler printer does not match the supplied printer")
	}
	if ph.layerSkipped.Load() {
		t.Error("expected layerSkipped to be false initially")
	}
	if ph.target != nil {
		t.Errorf("expected target to be nil initially, got %v", ph.target)
	}
	if !reflect.DeepEqual(ph.root, ocispec.Descriptor{}) {
		t.Errorf("expected root descriptor to be empty, got %v", ph.root)
	}
}

func TestPullHandler_OnFilePulled(t *testing.T) {
	handler := NewPullHandler(output.NewPrinter(&bytes.Buffer{}, os.Stderr))

	err := handler.OnFilePulled(
		"file.txt",
		"/tmp/file.txt",
		ocispec.Descriptor{Digest: "sha256:abc"},
		"application/octet-stream",
	)
	if err != nil {
		t.Fatalf("OnFilePulled() error = %v, want nil", err)
	}
}

func TestPullHandler_OnLayerSkipped(t *testing.T) {
	handler := NewPullHandler(output.NewPrinter(&bytes.Buffer{}, os.Stderr))
	ph := handler.(*PullHandler)

	if ph.layerSkipped.Load() {
		t.Fatal("expected layerSkipped to be false before OnLayerSkipped")
	}

	err := handler.OnLayerSkipped(ocispec.Descriptor{Digest: "sha256:abc"})
	if err != nil {
		t.Fatalf("OnLayerSkipped() error = %v, want nil", err)
	}

	if !ph.layerSkipped.Load() {
		t.Error("expected layerSkipped to be true after OnLayerSkipped")
	}
}

func TestPullHandler_OnPulled(t *testing.T) {
	handler := NewPullHandler(output.NewPrinter(&bytes.Buffer{}, os.Stderr))
	ph := handler.(*PullHandler)

	target := &option.Target{RawReference: "example.com/repo:v1"}
	desc := ocispec.Descriptor{
		Digest: "sha256:abc",
		Size:   123,
	}

	handler.OnPulled(target, desc)

	if ph.target != target {
		t.Error("OnPulled() did not store the supplied target")
	}
	if ph.root.Digest != desc.Digest {
		t.Errorf("OnPulled() root digest = %q, want %q", ph.root.Digest, desc.Digest)
	}
	if ph.root.Size != desc.Size {
		t.Errorf("OnPulled() root size = %d, want %d", ph.root.Size, desc.Size)
	}
}

func TestPullHandler_Render(t *testing.T) {
	tests := []struct {
		name       string
		skipLayer  bool
		wantOutput string
	}{
		{
			name: "pulled",
			wantOutput: "Pulled [] example.com/repo:v1\n" +
				"Digest: sha256:abc\n",
		},
		{
			name:      "layer skipped",
			skipLayer: true,
			wantOutput: "Skipped pulling layers without file name in " +
				"\"" + ocispec.AnnotationTitle + "\"\n" +
				"Use 'oras copy example.com/repo:v1 --to-oci-layout <layout-dir>' to pull all layers.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			printer := output.NewPrinter(&out, os.Stderr)
			handler := NewPullHandler(printer)

			target := &option.Target{RawReference: "example.com/repo:v1"}
			desc := ocispec.Descriptor{Digest: "sha256:abc"}

			handler.OnPulled(target, desc)
			if tt.skipLayer {
				if err := handler.OnLayerSkipped(ocispec.Descriptor{}); err != nil {
					t.Fatalf("OnLayerSkipped() error = %v", err)
				}
			}

			if err := handler.Render(); err != nil {
				t.Fatalf("Render() error = %v, want nil", err)
			}

			if got := out.String(); got != tt.wantOutput {
				t.Errorf("Render() output = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

// Ensure PullHandler satisfies the metadata.PullHandler interface.
var _ metadata.PullHandler = (*PullHandler)(nil)
