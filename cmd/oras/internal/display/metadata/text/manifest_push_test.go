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
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

func TestNewManifestPushHandler(t *testing.T) {
	printer := output.NewPrinter(&bytes.Buffer{}, os.Stderr)
	target := &option.Target{}

	handler := NewManifestPushHandler(printer, target)
	if handler == nil {
		t.Fatal("expected a non-nil handler")
	}

	mph, ok := handler.(*ManifestPushHandler)
	if !ok {
		t.Fatal("expected handler to be of type *ManifestPushHandler")
	}
	if mph.printer != printer {
		t.Error("handler printer does not match the supplied printer")
	}
	if mph.target != target {
		t.Error("handler target does not match the supplied target")
	}
	if !reflect.DeepEqual(mph.desc, ocispec.Descriptor{}) {
		t.Errorf("expected an empty descriptor, got %v", mph.desc)
	}
}

func TestManifestPushHandler_OnTagged(t *testing.T) {
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
			handler := NewManifestPushHandler(printer, &option.Target{})

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

func TestManifestPushHandler_OnManifestPushed(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		target     *option.Target
		desc       ocispec.Descriptor
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "manifest pushed",
			out:        &bytes.Buffer{},
			target:     &option.Target{RawReference: "example.com/repo:v1"},
			desc:       ocispec.Descriptor{Digest: "sha256:abc", Size: 123},
			wantOutput: "Pushed: [] example.com/repo:v1\n",
		},
		{
			name:    "error writer",
			out:     &errorWriter{},
			target:  &option.Target{RawReference: "example.com/repo:v1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewManifestPushHandler(printer, tt.target)

			err := handler.OnManifestPushed(tt.desc)
			if (err != nil) != tt.wantErr {
				t.Fatalf("OnManifestPushed() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !reflect.DeepEqual(handler.(*ManifestPushHandler).desc, tt.desc) {
				t.Errorf("stored descriptor = %v, want %v", handler.(*ManifestPushHandler).desc, tt.desc)
			}

			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.wantOutput {
					t.Errorf("OnManifestPushed() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestManifestPushHandler_Render(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		desc       ocispec.Descriptor
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "manifest pushed",
			out:        &bytes.Buffer{},
			desc:       ocispec.Descriptor{Digest: "sha256:abc"},
			wantOutput: "Digest: sha256:abc\n",
		},
		{
			name:    "error writer",
			out:     &errorWriter{},
			desc:    ocispec.Descriptor{Digest: "sha256:abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := &ManifestPushHandler{
				printer: printer,
				desc:    tt.desc,
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
