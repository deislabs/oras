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
	"testing"

	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

func TestNewManifestDeleteHandler(t *testing.T) {
	printer := output.NewPrinter(&bytes.Buffer{}, os.Stderr)
	target := &option.Target{
		Type:         "registry",
		RawReference: "localhost:5000/test:latest",
	}

	handler := NewManifestDeleteHandler(printer, target)
	if handler == nil {
		t.Fatal("expected a non-nil handler")
	}

	mdh, ok := handler.(*ManifestDeleteHandler)
	if !ok {
		t.Fatal("expected handler to be of type *ManifestDeleteHandler")
	}
	if mdh.printer != printer {
		t.Errorf("expected handler.printer to be %v, got %v", printer, mdh.printer)
	}
	if mdh.target != target {
		t.Errorf("expected handler.target to be %v, got %v", target, mdh.target)
	}
}

func TestManifestDeleteHandler_OnManifestMissing(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		target     *option.Target
		wantErr    bool
		wantOutput string
	}{
		{
			name: "digest reference",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			},
			wantOutput: "Missing localhost:5000/test@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n",
		},
		{
			name: "tag reference",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test:latest",
			},
			wantOutput: "Missing localhost:5000/test:latest\n",
		},
		{
			name: "OCI layout",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "oci-layout",
				RawReference: "./layout:latest",
			},
			wantOutput: "Missing ./layout:latest\n",
		},
		{
			name: "error writer",
			out:  &errorWriter{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test:latest",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewManifestDeleteHandler(printer, tt.target)

			err := handler.OnManifestMissing()
			if (err != nil) != tt.wantErr {
				t.Fatalf("OnManifestMissing() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.wantOutput {
					t.Errorf("OnManifestMissing() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestManifestDeleteHandler_OnManifestDeleted(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		target     *option.Target
		wantErr    bool
		wantOutput string
	}{
		{
			name: "digest reference",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			},
			wantOutput: "Deleted [registry] localhost:5000/test@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n",
		},
		{
			name: "tag reference",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test:latest",
			},
			wantOutput: "Deleted [registry] localhost:5000/test:latest\n",
		},
		{
			name: "OCI layout",
			out:  &bytes.Buffer{},
			target: &option.Target{
				Type:         "oci-layout",
				RawReference: "./layout:latest",
			},
			wantOutput: "Deleted [oci-layout] ./layout:latest\n",
		},
		{
			name: "error writer",
			out:  &errorWriter{},
			target: &option.Target{
				Type:         "registry",
				RawReference: "localhost:5000/test:latest",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewManifestDeleteHandler(printer, tt.target)

			err := handler.OnManifestDeleted()
			if (err != nil) != tt.wantErr {
				t.Fatalf("OnManifestDeleted() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				got := tt.out.(*bytes.Buffer).String()
				if got != tt.wantOutput {
					t.Errorf("OnManifestDeleted() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}
