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

package root

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/oci"

	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/option"
)

func Test_attachCmd_configAndPlatformMutuallyExclusive(t *testing.T) {
	cmd := attachCmd()
	cmd.SetArgs([]string{
		"--artifact-type", "doc/example",
		"--config", "config.json",
		"--platform", "linux/amd64",
		"localhost:5000/hello:v1",
		"hi.txt",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be used at the same time") {
		t.Fatalf("expected mutual exclusion error, got %v", err)
	}
}

func Test_attachCmd_configFlagRegistered(t *testing.T) {
	cmd := attachCmd()
	f := cmd.Flags().Lookup("config")
	if f == nil {
		t.Fatal("expected --config flag to be registered")
	}
	if f.Usage != "`path` of image config file" {
		t.Fatalf("unexpected usage: %q", f.Usage)
	}
}

func Test_runAttach_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test
	opts := &attachOptions{
		Packer: option.Packer{
			Annotation: option.Annotation{
				ManifestAnnotations: []string{"one", "two"},
			},
			AnnotationFilePath: "/tmp/whatever",
		},
	}
	got := opts.Packer.Parse(cmd).Error()
	want := errors.New("`--annotation` and `--annotation-file` cannot be both specified").Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_attachCmd_configAnnotationsWiredToPackOpts(t *testing.T) {
	// Regression for #2022: config annotations from an annotation file must
	// flow through to oras.PackManifestOptions.ConfigAnnotations.
	annotationFile := filepath.Join(t.TempDir(), "annot.json")
	content := `{"$config":{"org.example.key":"val"}}`
	if err := os.WriteFile(annotationFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	opts := &attachOptions{}
	opts.AnnotationFilePath = annotationFile
	if err := opts.Packer.Parse(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	packOpts := buildAttachPackOpts(opts, ocispec.Descriptor{}, nil)
	if packOpts.ConfigAnnotations == nil {
		t.Fatal("expected ConfigAnnotations to be populated; got nil")
	}
	if packOpts.ConfigAnnotations["org.example.key"] != "val" {
		t.Fatalf("expected 'org.example.key'='val', got %v", packOpts.ConfigAnnotations)
	}
}

func Test_doAttach_handlerCalls(t *testing.T) {
	ctx := context.Background()
	dst, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	subject, err := oras.PackManifest(ctx, dst, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := file.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	fileRef := filepath.Join(t.TempDir(), "layer.txt")
	if err := os.WriteFile(fileRef, []byte("layer"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := &attachOptions{
		Packer:       option.Packer{FileRefs: []string{fileRef}},
		artifactType: "application/example.artifact",
	}
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingAttachStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingAttachMetadataHandler{Recorder: recorder}

	if err := doAttach(ctx, opts, store, dst, subject, statusHandler, metadataHandler); err != nil {
		t.Fatal(err)
	}

	calls := recorder.Snapshot()
	if len(calls) < 5 {
		t.Fatalf("handler calls = %v, want at least 5", calls)
	}
	if got, want := calls[:2], []string{"OnFileLoading", "TrackTarget"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first handler calls = %v, want %v", got, want)
	}
	if got, want := calls[len(calls)-3:], []string{"StopTrackTarget", "OnAttached", "Render"}; !reflect.DeepEqual(got, want) {
		t.Errorf("last handler calls = %v, want %v", got, want)
	}
}
