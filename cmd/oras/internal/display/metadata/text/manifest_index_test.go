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
	"io"
	"os"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/output"
)

func TestNewManifestIndexCreateHandler(t *testing.T) {
	printer := output.NewPrinter(&bytes.Buffer{}, os.Stderr)

	handler := NewManifestIndexCreateHandler(printer)
	if handler == nil {
		t.Fatal("expected a non-nil handler")
	}

	mich, ok := handler.(*ManifestIndexCreateHandler)
	if !ok {
		t.Fatal("expected handler to be of type *ManifestIndexCreateHandler")
	}
	if mich.printer != printer {
		t.Errorf("expected handler.printer to be %v, got %v", printer, mich.printer)
	}
	if !reflect.DeepEqual(mich.root, ocispec.Descriptor{}) {
		t.Errorf("expected handler.root to be empty, got %v", mich.root)
	}
}

func TestManifestIndexCreateHandler_OnTagged(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		tag        string
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "tag",
			out:        &bytes.Buffer{},
			tag:        "v1",
			wantOutput: "Tagged v1\n",
		},
		{
			name:    "error writer",
			out:     &errorWriter{},
			tag:     "v1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewManifestIndexCreateHandler(printer)

			err := handler.OnTagged(ocispec.Descriptor{}, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Fatalf("OnTagged() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.wantOutput {
					t.Errorf("OnTagged() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestManifestIndexCreateHandler_OnIndexCreated(t *testing.T) {
	handler := &ManifestIndexCreateHandler{}
	desc := ocispec.Descriptor{
		Digest: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		Size:   123,
	}

	handler.OnIndexCreated(desc)

	if !reflect.DeepEqual(handler.root, desc) {
		t.Errorf("OnIndexCreated() root = %v, want %v", handler.root, desc)
	}
}

func TestManifestIndexCreateHandler_Render(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		root       ocispec.Descriptor
		wantOutput string
		wantErr    bool
	}{
		{
			name: "index created",
			out:  &bytes.Buffer{},
			root: ocispec.Descriptor{
				Digest: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			},
			wantOutput: "Digest: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n",
		},
		{
			name:    "error writer",
			out:     &errorWriter{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := &ManifestIndexCreateHandler{
				printer: printer,
				root:    tt.root,
			}

			err := handler.Render()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Render() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.wantOutput {
					t.Errorf("Render() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}
