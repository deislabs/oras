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
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

type errorWriter struct{}

// Write implements the io.Writer interface and returns an error in Write.
func (w *errorWriter) Write(_ []byte) (n int, err error) {
	return 0, fmt.Errorf("got an error")
}

func TestPushHandler_Render(t *testing.T) {
	content := []byte("content")
	tests := []struct {
		name    string
		out     io.Writer
		root    ocispec.Descriptor
		wantErr bool
	}{
		{
			"good path",
			&bytes.Buffer{},
			ocispec.Descriptor{
				MediaType: "example",
				Digest:    digest.FromBytes(content),
				Size:      int64(len(content)),
			},
			false,
		},
		{
			"error path",
			&errorWriter{},
			ocispec.Descriptor{
				MediaType: "example",
				Digest:    digest.FromBytes(content),
				Size:      int64(len(content)),
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			p := &PushHandler{
				printer: printer,
				root:    tt.root,
			}
			if err := p.Render(); (err != nil) != tt.wantErr {
				t.Fatalf("PushHandler.Render() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewPushHandler(t *testing.T) {
	var out bytes.Buffer
	printer := output.NewPrinter(&out, os.Stderr)

	handler := NewPushHandler(printer)
	if handler == nil {
		t.Fatal("expected a non-nil handler")
	}

	ph, ok := handler.(*PushHandler)
	if !ok {
		t.Fatal("expected handler to be of type *PushHandler")
	}
	if ph.printer != printer {
		t.Error("handler printer does not match the supplied printer")
	}
}

func TestPushHandler_OnTagged(t *testing.T) {
	tests := []struct {
		name       string
		out        io.Writer
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "success",
			out:        &bytes.Buffer{},
			wantOutput: "Tagged v1\n",
		},
		{
			name:    "writer error",
			out:     &errorWriter{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			printer := output.NewPrinter(tt.out, os.Stderr)
			handler := NewPushHandler(printer)

			err := handler.OnTagged(ocispec.Descriptor{}, "v1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("OnTagged() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if got := tt.out.(*bytes.Buffer).String(); got != tt.wantOutput {
					t.Errorf("OnTagged() output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestPushHandler_OnCopied(t *testing.T) {
	var out bytes.Buffer
	printer := output.NewPrinter(&out, os.Stderr)
	handler := NewPushHandler(printer)

	target := &option.Target{RawReference: "example.com/repo:v1"}
	root := ocispec.Descriptor{
		ArtifactType: "application/example",
		Digest:       digest.FromBytes([]byte("content")),
		Size:         int64(len("content")),
	}

	err := handler.OnCopied(target, root)
	if err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}

	wantOutput := "Pushed [] example.com/repo:v1\n"
	if got := out.String(); got != wantOutput {
		t.Errorf("OnCopied() output = %q, want %q", got, wantOutput)
	}

	ph := handler.(*PushHandler)
	if ph.root.ArtifactType != root.ArtifactType {
		t.Errorf("stored ArtifactType = %q, want %q", ph.root.ArtifactType, root.ArtifactType)
	}
	if ph.root.Digest != root.Digest {
		t.Errorf("stored Digest = %q, want %q", ph.root.Digest, root.Digest)
	}
}
