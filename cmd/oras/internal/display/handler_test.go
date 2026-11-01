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

package display

import (
	"os"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/internal/testutils"

	"oras.land/oras/cmd/oras/internal/display/content"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/metadata/descriptor"
	"oras.land/oras/cmd/oras/internal/display/metadata/json"
	"oras.land/oras/cmd/oras/internal/display/metadata/table"
	"oras.land/oras/cmd/oras/internal/display/metadata/template"
	"oras.land/oras/cmd/oras/internal/display/metadata/text"
	"oras.land/oras/cmd/oras/internal/display/metadata/tree"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/internal/output"
)

func TestNewPushHandler(t *testing.T) {
	mockFetcher := testutils.NewMockFetcher()
	printer := output.NewPrinter(os.Stdout, os.Stderr)

	tests := []struct {
		name           string
		format         option.Format
		tty            *os.File
		wantStatusType reflect.Type
		wantMetaType   reflect.Type
		wantError      bool
	}{
		{
			name:           "text",
			format:         option.Format{Type: option.FormatTypeText.Name},
			wantStatusType: reflect.TypeOf(&status.TextPushHandler{}),
			wantMetaType:   reflect.TypeOf(&text.PushHandler{}),
		},
		{
			name:           "text with TTY",
			format:         option.Format{Type: option.FormatTypeText.Name},
			tty:            os.Stdout,
			wantStatusType: reflect.TypeOf(&status.TTYPushHandler{}),
			wantMetaType:   reflect.TypeOf(&text.PushHandler{}),
		},
		{
			name:           "JSON",
			format:         option.Format{Type: option.FormatTypeJSON.Name},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(&json.PushHandler{}),
		},
		{
			name:           "JSON with TTY",
			format:         option.Format{Type: option.FormatTypeJSON.Name},
			tty:            os.Stdout,
			wantStatusType: reflect.TypeOf(&status.TTYPushHandler{}),
			wantMetaType:   reflect.TypeOf(&json.PushHandler{}),
		},
		{
			name:           "Go template",
			format:         option.Format{Type: option.FormatTypeGoTemplate.Name, Template: "{{.name}}"},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(&template.PushHandler{}),
		},
		{
			name:      "unsupported format",
			format:    option.Format{Type: "unsupported"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusHandler, metadataHandler, err := NewPushHandler(
				printer, tt.format, tt.tty, mockFetcher.Fetcher,
			)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if statusHandler != nil || metadataHandler != nil {
					t.Fatal("expected nil handlers for unsupported format")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewPushHandler() error = %v", err)
			}
			if got := reflect.TypeOf(statusHandler); got != tt.wantStatusType {
				t.Errorf("status handler type = %v, want %v", got, tt.wantStatusType)
			}
			if got := reflect.TypeOf(metadataHandler); got != tt.wantMetaType {
				t.Errorf("metadata handler type = %v, want %v", got, tt.wantMetaType)
			}
		})
	}
}

func TestNewAttachHandler(t *testing.T) {
	mockFetcher := testutils.NewMockFetcher()
	printer := output.NewPrinter(os.Stdout, os.Stderr)

	tests := []struct {
		name           string
		format         option.Format
		tty            *os.File
		wantStatusType reflect.Type
		wantMetaType   reflect.Type
		wantErr        bool
	}{
		{
			name:           "text",
			format:         option.Format{Type: option.FormatTypeText.Name},
			wantStatusType: reflect.TypeOf((*status.TextPushHandler)(nil)),
			wantMetaType:   reflect.TypeOf(text.NewAttachHandler(printer)),
		},
		{
			name:           "text with tty",
			format:         option.Format{Type: option.FormatTypeText.Name},
			tty:            os.Stdout,
			wantStatusType: reflect.TypeOf((*status.TTYPushHandler)(nil)),
			wantMetaType:   reflect.TypeOf(text.NewAttachHandler(printer)),
		},
		{
			name:           "json",
			format:         option.Format{Type: option.FormatTypeJSON.Name},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(json.NewAttachHandler(printer)),
		},
		{
			name:           "template",
			format:         option.Format{Type: option.FormatTypeGoTemplate.Name},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(template.NewAttachHandler(printer, "")),
		},
		{
			name:    "unsupported format",
			format:  option.Format{Type: "unsupported"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusHandler, metadataHandler, err := NewAttachHandler(
				printer, tt.format, tt.tty, mockFetcher.Fetcher,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error for unsupported format")
				}
				if statusHandler != nil || metadataHandler != nil {
					t.Fatal("expected nil handlers for unsupported format")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAttachHandler() error = %v", err)
			}
			if got := reflect.TypeOf(statusHandler); got != tt.wantStatusType {
				t.Errorf("status handler type = %v, want %v", got, tt.wantStatusType)
			}
			if got := reflect.TypeOf(metadataHandler); got != tt.wantMetaType {
				t.Errorf("metadata handler type = %v, want %v", got, tt.wantMetaType)
			}
		})
	}
}

func TestNewPullHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)

	tests := []struct {
		name           string
		format         option.Format
		tty            *os.File
		wantStatusType reflect.Type
		wantMetaType   reflect.Type
		wantErr        bool
	}{
		{
			name:           "text",
			format:         option.Format{Type: option.FormatTypeText.Name},
			wantStatusType: reflect.TypeOf((*status.TextPullHandler)(nil)),
			wantMetaType:   reflect.TypeOf(text.NewPullHandler(printer)),
		},
		{
			name:           "text with tty",
			format:         option.Format{Type: option.FormatTypeText.Name},
			tty:            os.Stdout,
			wantStatusType: reflect.TypeOf((*status.TTYPullHandler)(nil)),
			wantMetaType:   reflect.TypeOf(text.NewPullHandler(printer)),
		},
		{
			name:           "json",
			format:         option.Format{Type: option.FormatTypeJSON.Name},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(json.NewPullHandler(printer, "")),
		},
		{
			name:           "template",
			format:         option.Format{Type: option.FormatTypeGoTemplate.Name},
			wantStatusType: reflect.TypeOf(status.DiscardHandler{}),
			wantMetaType:   reflect.TypeOf(template.NewPullHandler(printer, "", "")),
		},
		{
			name:    "unsupported format",
			format:  option.Format{Type: "unsupported"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusHandler, metadataHandler, err := NewPullHandler(
				printer, tt.format, "", tt.tty,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error for unsupported format")
				}
				if statusHandler != nil || metadataHandler != nil {
					t.Fatal("expected nil handlers for unsupported format")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewPullHandler() error = %v", err)
			}
			if got := reflect.TypeOf(statusHandler); got != tt.wantStatusType {
				t.Errorf("status handler type = %v, want %v", got, tt.wantStatusType)
			}
			if got := reflect.TypeOf(metadataHandler); got != tt.wantMetaType {
				t.Errorf("metadata handler type = %v, want %v", got, tt.wantMetaType)
			}
		})
	}
}

func TestNewCopyHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	copyHandler, copyMetadataHandler := NewCopyHandler(printer, os.Stdout, nil)
	if _, ok := copyHandler.(*status.TTYCopyHandler); !ok {
		t.Errorf("expected *status.TTYCopyHandler actual %v", reflect.TypeOf(copyHandler))
	}
	if _, ok := copyMetadataHandler.(*text.CopyHandler); !ok {
		t.Errorf("expected metadata.CopyHandler actual %v", reflect.TypeOf(copyMetadataHandler))
	}
	copyHandler, copyMetadataHandler = NewCopyHandler(printer, nil, nil)
	if _, ok := copyHandler.(*status.TextCopyHandler); !ok {
		t.Errorf("expected *status.TextCopyHandler actual %v", reflect.TypeOf(copyHandler))
	}
	if _, ok := copyMetadataHandler.(*text.CopyHandler); !ok {
		t.Errorf("expected metadata.CopyHandler actual %v", reflect.TypeOf(copyMetadataHandler))
	}
}

func TestNewRepoTagsHandler(t *testing.T) {
	out := os.Stdout

	tests := []struct {
		name     string
		format   option.Format
		wantType reflect.Type
		wantErr  bool
	}{
		{
			name:     "text",
			format:   option.Format{Type: option.FormatTypeText.Name},
			wantType: reflect.TypeOf(text.NewRepoTagsHandler(out)),
		},
		{
			name:     "json",
			format:   option.Format{Type: option.FormatTypeJSON.Name},
			wantType: reflect.TypeOf(json.NewRepoTagsHandler(out)),
		},
		{
			name:     "template",
			format:   option.Format{Type: option.FormatTypeGoTemplate.Name},
			wantType: reflect.TypeOf(template.NewRepoTagsHandler(out, "")),
		},
		{
			name:    "unsupported format",
			format:  option.Format{Type: "unsupported"},
			wantErr: true,
		},
	}

	// Test with stdout
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewRepoTagsHandler(out, tt.format)

			if tt.wantErr {
				if err == nil {
					t.Fatal("NewRepoTagsHandler() error = nil, want error")
				}
				if handler != nil {
					t.Fatal("NewRepoTagsHandler() handler != nil, want nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewRepoTagsHandler() error = %v, want nil", err)
			}
			if got := reflect.TypeOf(handler); got != tt.wantType {
				t.Errorf("handler type = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestNewRepoListHandler(t *testing.T) {
	out := os.Stdout
	registry := "registry.example.com"
	namespace := "example"

	tests := []struct {
		name     string
		format   option.Format
		wantType reflect.Type
		wantErr  bool
	}{
		{
			name:     "text",
			format:   option.Format{Type: option.FormatTypeText.Name},
			wantType: reflect.TypeOf(text.NewRepoListHandler(out, namespace)),
		},
		{
			name:     "json",
			format:   option.Format{Type: option.FormatTypeJSON.Name},
			wantType: reflect.TypeOf(json.NewRepoListHandler(out, registry)),
		},
		{
			name:     "template",
			format:   option.Format{Type: option.FormatTypeGoTemplate.Name},
			wantType: reflect.TypeOf(template.NewRepoListHandler(out, "", registry)),
		},
		{
			name:    "unsupported format",
			format:  option.Format{Type: "unsupported"},
			wantErr: true,
		},
	}

	// Test with stdout
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewRepoListHandler(out, tt.format, registry, namespace)

			if tt.wantErr {
				if err == nil {
					t.Fatal("NewRepoListHandler() error = nil, want error")
				}
				if handler != nil {
					t.Fatal("NewRepoListHandler() handler != nil, want nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewRepoListHandler() error = %v, want nil", err)
			}
			if got := reflect.TypeOf(handler); got != tt.wantType {
				t.Errorf("handler type = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestNewBackupHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	repo := "test/repo"
	mockFetcher := testutils.NewMockFetcher()

	t.Run("with TTY", func(t *testing.T) {
		statusHandler, metadataHandler := NewBackupHandler(printer, os.Stdout, repo, mockFetcher.Fetcher)
		if _, ok := statusHandler.(*status.TTYBackupHandler); !ok {
			t.Errorf("expected *status.TTYBackupHandler actual %v", reflect.TypeOf(statusHandler))
		}
		if _, ok := metadataHandler.(*text.BackupHandler); !ok {
			t.Errorf("expected *text.BackupHandler actual %v", reflect.TypeOf(metadataHandler))
		}
	})

	t.Run("without TTY", func(t *testing.T) {
		statusHandler, metadataHandler := NewBackupHandler(printer, nil, repo, mockFetcher.Fetcher)
		if _, ok := statusHandler.(*status.TextBackupHandler); !ok {
			t.Errorf("expected *status.TextBackupHandler actual %v", reflect.TypeOf(statusHandler))
		}
		if _, ok := metadataHandler.(*text.BackupHandler); !ok {
			t.Errorf("expected *text.BackupHandler actual %v", reflect.TypeOf(metadataHandler))
		}
	})
}

func TestNewRestoreHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	mockFetcher := testutils.NewMockFetcher()

	t.Run("with TTY", func(t *testing.T) {
		statusHandler, metadataHandler := NewRestoreHandler(printer, os.Stdout, mockFetcher.Fetcher, false)
		if _, ok := statusHandler.(*status.TTYRestoreHandler); !ok {
			t.Errorf("expected *status.TTYRestoreHandler actual %v", reflect.TypeOf(statusHandler))
		}
		if _, ok := metadataHandler.(*text.RestoreHandler); !ok {
			t.Errorf("expected *text.RestoreHandler actual %v", reflect.TypeOf(metadataHandler))
		}
	})

	t.Run("without TTY", func(t *testing.T) {
		statusHandler, metadataHandler := NewRestoreHandler(printer, nil, mockFetcher.Fetcher, false)
		if _, ok := statusHandler.(*status.TextRestoreHandler); !ok {
			t.Errorf("expected *status.TextRestoreHandler actual %v", reflect.TypeOf(statusHandler))
		}
		if _, ok := metadataHandler.(*text.RestoreHandler); !ok {
			t.Errorf("expected *text.RestoreHandler actual %v", reflect.TypeOf(metadataHandler))
		}
	})
}

func TestNewManifestFetchHandler(t *testing.T) {
	out := os.Stdout
	tests := []struct {
		name             string
		format           option.Format
		outputDescriptor bool
		outputPath       string
		wantMetadataType reflect.Type
		wantContentType  reflect.Type
	}{
		{
			name:             "text descriptor",
			format:           option.Format{Type: option.FormatTypeText.Name},
			outputDescriptor: true,
			wantMetadataType: reflect.TypeOf(descriptor.NewManifestFetchHandler(out, false)),
			wantContentType:  reflect.TypeOf(content.NewManifestFetchHandler(out, false, "")),
		},
		{
			name:             "json stdout",
			format:           option.Format{Type: option.FormatTypeJSON.Name},
			wantMetadataType: reflect.TypeOf(json.NewManifestFetchHandler(out)),
			wantContentType:  reflect.TypeOf(content.NewDiscardHandler()),
		},
		{
			name:             "template file",
			format:           option.Format{Type: option.FormatTypeGoTemplate.Name, Template: "{{.digest}}"},
			outputPath:       "manifest.json",
			wantMetadataType: reflect.TypeOf(template.NewManifestFetchHandler(out, "{{.digest}}")),
			wantContentType:  reflect.TypeOf(content.NewManifestFetchHandler(out, false, "manifest.json")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataHandler, contentHandler, err := NewManifestFetchHandler(out, tt.format, tt.outputDescriptor, false, tt.outputPath)
			if err != nil {
				t.Fatalf("NewManifestFetchHandler() error = %v", err)
			}
			if got := reflect.TypeOf(metadataHandler); got != tt.wantMetadataType {
				t.Errorf("metadata handler type = %v, want %v", got, tt.wantMetadataType)
			}
			if got := reflect.TypeOf(contentHandler); got != tt.wantContentType {
				t.Errorf("content handler type = %v, want %v", got, tt.wantContentType)
			}
		})
	}
}

func TestNewDiscoverHandler(t *testing.T) {
	out := os.Stdout
	desc := ocispec.Descriptor{}
	tests := []struct {
		name     string
		format   option.Format
		tty      *os.File
		wantType reflect.Type
	}{
		{name: "tree", format: option.Format{Type: option.FormatTypeTree.Name}, wantType: reflect.TypeOf(tree.NewDiscoverHandler(out, "", desc, false, nil))},
		{name: "tree with TTY", format: option.Format{Type: option.FormatTypeTree.Name}, tty: os.Stdout, wantType: reflect.TypeOf(tree.NewDiscoverHandler(out, "", desc, false, os.Stdout))},
		{name: "table", format: option.Format{Type: option.FormatTypeTable.Name}, wantType: reflect.TypeOf(table.NewDiscoverHandler(out, "ref", desc, false))},
		{name: "json", format: option.Format{Type: option.FormatTypeJSON.Name}, wantType: reflect.TypeOf(json.NewDiscoverHandler(out, desc, ""))},
		{name: "template", format: option.Format{Type: option.FormatTypeGoTemplate.Name, Template: "{{.digest}}"}, wantType: reflect.TypeOf(template.NewDiscoverHandler(out, desc, "", "{{.digest}}"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDiscoverHandler(out, tt.format, "", "ref", desc, false, tt.tty)
			if err != nil {
				t.Fatalf("NewDiscoverHandler() error = %v", err)
			}
			if gotType := reflect.TypeOf(got); gotType != tt.wantType {
				t.Errorf("handler type = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

func TestNewTagHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	handler := NewTagHandler(printer, option.Target{Reference: "example:v1"})
	if got, want := reflect.TypeOf(handler), reflect.TypeOf(text.NewTagHandler(printer, option.Target{Reference: "example:v1"})); got != want {
		t.Errorf("handler type = %v, want %v", got, want)
	}
}

func TestNewManifestDeleteHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	target := &option.Target{Reference: "example:v1"}
	handler := NewManifestDeleteHandler(printer, target)
	if got, want := reflect.TypeOf(handler), reflect.TypeOf(text.NewManifestDeleteHandler(printer, target)); got != want {
		t.Errorf("handler type = %v, want %v", got, want)
	}
}

func TestNewResolveHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	handler := NewResolveHandler(printer, true, "descriptor.json")
	if got, want := reflect.TypeOf(handler), reflect.TypeOf(text.NewResolveHandler(printer, true, "descriptor.json")); got != want {
		t.Errorf("handler type = %v, want %v", got, want)
	}
}

func TestNewBlobDeleteHandler(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	target := &option.Target{Reference: "example:v1"}
	handler := NewBlobDeleteHandler(printer, target)
	if got, want := reflect.TypeOf(handler), reflect.TypeOf(text.NewBlobDeleteHandler(printer, target)); got != want {
		t.Errorf("handler type = %v, want %v", got, want)
	}
}

func TestNewBlobPushHandler_outputDescriptor(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	statusHandler, metadataHandler := NewBlobPushHandler(printer, true, false, ocispec.Descriptor{}, nil)
	if got, want := reflect.TypeOf(statusHandler), reflect.TypeOf(status.NewDiscardHandler()); got != want {
		t.Errorf("status handler type = %v, want %v", got, want)
	}
	if got, want := reflect.TypeOf(metadataHandler), reflect.TypeOf(metadata.NewDiscardHandler()); got != want {
		t.Errorf("metadata handler type = %v, want %v", got, want)
	}
}

func TestNewBlobPushHandler_output(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	desc := ocispec.Descriptor{}
	tests := []struct {
		name             string
		tty              *os.File
		wantStatusType   reflect.Type
		wantMetadataType reflect.Type
	}{
		{name: "text", wantStatusType: reflect.TypeOf(status.NewTextBlobPushHandler(printer, desc)), wantMetadataType: reflect.TypeOf(text.NewBlobPushHandler(printer, desc))},
		{name: "TTY", tty: os.Stdout, wantStatusType: reflect.TypeOf(status.NewTTYBlobPushHandler(os.Stdout, desc)), wantMetadataType: reflect.TypeOf(text.NewBlobPushHandler(printer, desc))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMetadata := NewBlobPushHandler(printer, false, false, desc, tt.tty)
			if got := reflect.TypeOf(gotStatus); got != tt.wantStatusType {
				t.Errorf("status handler type = %v, want %v", got, tt.wantStatusType)
			}
			if got := reflect.TypeOf(gotMetadata); got != tt.wantMetadataType {
				t.Errorf("metadata handler type = %v, want %v", got, tt.wantMetadataType)
			}
		})
	}
}

func TestNewManifestPushHandler_outputDescriptor(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	statusHandler, metadataHandler := NewManifestPushHandler(printer, true, false, ocispec.Descriptor{}, nil)
	if got, want := reflect.TypeOf(statusHandler), reflect.TypeOf(status.NewDiscardHandler()); got != want {
		t.Errorf("status handler type = %v, want %v", got, want)
	}
	if got, want := reflect.TypeOf(metadataHandler), reflect.TypeOf(metadata.NewDiscardHandler()); got != want {
		t.Errorf("metadata handler type = %v, want %v", got, want)
	}
}

func TestNewManifestPushHandler_output(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	desc := ocispec.Descriptor{}
	target := &option.Target{Reference: "example:v1"}
	statusHandler, metadataHandler := NewManifestPushHandler(printer, false, false, desc, target)
	if got, want := reflect.TypeOf(statusHandler), reflect.TypeOf(status.NewTextManifestPushHandler(printer, desc)); got != want {
		t.Errorf("status handler type = %v, want %v", got, want)
	}
	if got, want := reflect.TypeOf(metadataHandler), reflect.TypeOf(text.NewManifestPushHandler(printer, target)); got != want {
		t.Errorf("metadata handler type = %v, want %v", got, want)
	}
}

func TestNewManifestIndexCreateHandler_outputModes(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	tests := []struct {
		name         string
		outputPath   string
		statusType   reflect.Type
		metadataType reflect.Type
		contentType  reflect.Type
	}{
		{
			name:         "stdout",
			statusType:   reflect.TypeOf(status.NewTextManifestIndexCreateHandler(printer)),
			metadataType: reflect.TypeOf(text.NewManifestIndexCreateHandler(printer)),
			contentType:  reflect.TypeOf(content.NewDiscardHandler()),
		},
		{
			name:         "raw stdout",
			outputPath:   "-",
			statusType:   reflect.TypeOf(status.NewDiscardHandler()),
			metadataType: reflect.TypeOf(metadata.NewDiscardHandler()),
			contentType:  reflect.TypeOf(content.NewManifestIndexCreateHandler(printer, false, "-")),
		},
		{
			name:         "file",
			outputPath:   "index.json",
			statusType:   reflect.TypeOf(status.NewTextManifestIndexCreateHandler(printer)),
			metadataType: reflect.TypeOf(text.NewManifestIndexCreateHandler(printer)),
			contentType:  reflect.TypeOf(content.NewManifestIndexCreateHandler(printer, false, "index.json")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMetadata, gotContent := NewManifestIndexCreateHandler(tt.outputPath, printer, false)
			if reflect.TypeOf(gotStatus) != tt.statusType || reflect.TypeOf(gotMetadata) != tt.metadataType || reflect.TypeOf(gotContent) != tt.contentType {
				t.Errorf("handler types = (%T, %T, %T), want (%v, %v, %v)", gotStatus, gotMetadata, gotContent, tt.statusType, tt.metadataType, tt.contentType)
			}
		})
	}
}

func TestNewManifestIndexUpdateHandler_outputModes(t *testing.T) {
	printer := output.NewPrinter(os.Stdout, os.Stderr)
	tests := []struct {
		name         string
		outputPath   string
		statusType   reflect.Type
		metadataType reflect.Type
		contentType  reflect.Type
	}{
		{
			name:         "stdout",
			statusType:   reflect.TypeOf(status.NewTextManifestIndexUpdateHandler(printer)),
			metadataType: reflect.TypeOf(text.NewManifestIndexCreateHandler(printer)),
			contentType:  reflect.TypeOf(content.NewDiscardHandler()),
		},
		{
			name:         "raw stdout",
			outputPath:   "-",
			statusType:   reflect.TypeOf(status.NewDiscardHandler()),
			metadataType: reflect.TypeOf(metadata.NewDiscardHandler()),
			contentType:  reflect.TypeOf(content.NewManifestIndexCreateHandler(printer, false, "-")),
		},
		{
			name:         "file",
			outputPath:   "index.json",
			statusType:   reflect.TypeOf(status.NewTextManifestIndexUpdateHandler(printer)),
			metadataType: reflect.TypeOf(text.NewManifestIndexCreateHandler(printer)),
			contentType:  reflect.TypeOf(content.NewManifestIndexCreateHandler(printer, false, "index.json")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMetadata, gotContent := NewManifestIndexUpdateHandler(tt.outputPath, printer, false)
			if reflect.TypeOf(gotStatus) != tt.statusType || reflect.TypeOf(gotMetadata) != tt.metadataType || reflect.TypeOf(gotContent) != tt.contentType {
				t.Errorf("handler types = (%T, %T, %T), want (%v, %v, %v)", gotStatus, gotMetadata, gotContent, tt.statusType, tt.metadataType, tt.contentType)
			}
		})
	}
}
