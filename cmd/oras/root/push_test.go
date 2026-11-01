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
	stderrors "errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/contentutil"
)

func Test_runPush_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test
	opts := &pushOptions{
		Format: option.Format{
			Type: "unknown",
		},
	}
	got := runPush(cmd, opts).Error()
	want := errors.UnsupportedFormatTypeError(opts.Format.Type).Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Test_runPush_force exercises pushing with --force, including the extra-tag
// path that uses the TraversingTarget-wrapped destination.
func Test_runPush_force(t *testing.T) {
	// prepare a file to push
	tempDir := t.TempDir()
	fileName := "hi.txt"
	filePath := filepath.Join(tempDir, fileName)
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}

	// use an OCI image layout as the push destination
	layoutDir := filepath.Join(tempDir, "layout")

	cmd := pushCmd()
	cmd.SetArgs([]string{
		"--oci-layout",
		"--force",
		"--disable-path-validation",
		layoutDir + ":tag1,tag2",
		filePath,
	})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify the artifact and both tags were pushed to the layout
	store, err := oci.New(layoutDir)
	if err != nil {
		t.Fatalf("failed to open pushed layout: %v", err)
	}
	for _, tag := range []string{"tag1", "tag2"} {
		if _, err := store.Resolve(context.Background(), tag); err != nil {
			t.Errorf("expected tag %q to be resolvable, got error: %v", tag, err)
		}
	}
}

func Test_pushCmd_invalidExtraTag(t *testing.T) {
	cmd := pushCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1,,invalid@@tag",
		"hi.txt",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != "empty extra tag" {
		t.Fatalf("got %v, want empty extra tag error", err)
	}
}

func Test_doPushCommand_fileLoadingError(t *testing.T) {
	ctx := context.Background()
	wantErr := stderrors.New("file loading failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{
		Recorder:       recorder,
		FileLoadingErr: wantErr,
	}

	opts := &pushOptions{}
	opts.FileRefs = []string{"unused.txt"}

	targetCreated := false
	err := doPushCommand(
		ctx,
		opts,
		oras.PackManifestOptions{},
		nil, // store is never used: OnFileLoading fails before addFile
		nil,
		nil,
		statusHandler,
		&displaytest.RecordingPushMetadataHandler{Recorder: recorder},
		func() (oras.GraphTarget, error) {
			targetCreated = true
			return nil, stderrors.New("unexpected target creation")
		},
	)
	if !stderrors.Is(err, wantErr) {
		t.Fatalf("doPushCommand() error = %v, want %v", err, wantErr)
	}
	if targetCreated {
		t.Fatal("destination was created after file loading failed")
	}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, []string{"OnFileLoading"}) {
		t.Fatalf("handler calls = %v, want [OnFileLoading]", got)
	}
}

func Test_doPushCommand_displayHandlerCalls(t *testing.T) {
	ctx := context.Background()

	filePath := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}

	store, err := file.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	memoryStore := memory.New()
	union := contentutil.MultiReadOnlyTarget(memoryStore, store)
	destination := memory.New()

	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingPushMetadataHandler{Recorder: recorder}

	opts := &pushOptions{}
	opts.FileRefs = []string{filePath}
	opts.Reference = "tag1"
	opts.PackVersion = oras.PackManifestVersion1_1
	opts.artifactType = "application/example"
	opts.extraRefs = []string{"tag2"}

	err = doPushCommand(
		ctx,
		opts,
		oras.PackManifestOptions{},
		store,
		memoryStore,
		union,
		statusHandler,
		metadataHandler,
		func() (oras.GraphTarget, error) {
			return destination, nil
		},
	)
	if err != nil {
		t.Fatalf("doPushCommand() error = %v", err)
	}

	wantCalls := []string{"OnFileLoading", "OnCopied", "OnTagged:tag2", "Render"}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("handler calls = %v, want %v", got, wantCalls)
	}
}

func Test_doPushCommand_onCopiedError(t *testing.T) {
	ctx := context.Background()
	recorder := &displaytest.CallRecorder{}
	wantErr := stderrors.New("OnCopied failed")
	metadataHandler := &displaytest.RecordingPushMetadataHandler{
		Recorder:  recorder,
		CopiedErr: wantErr,
	}

	store, err := file.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	memoryStore := memory.New()
	union := contentutil.MultiReadOnlyTarget(memoryStore, store)

	filePath := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}

	exportPath := filepath.Join(t.TempDir(), "manifest.json")
	opts := &pushOptions{}
	opts.FileRefs = []string{filePath}
	opts.Reference = "tag1"
	opts.PackVersion = oras.PackManifestVersion1_1
	opts.artifactType = "application/example"
	opts.extraRefs = []string{"tag2"}
	opts.ManifestExportPath = exportPath

	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder}
	err = doPushCommand(
		ctx,
		opts,
		oras.PackManifestOptions{},
		store,
		memoryStore,
		union,
		statusHandler,
		metadataHandler,
		func() (oras.GraphTarget, error) {
			return memory.New(), nil
		},
	)
	if !stderrors.Is(err, wantErr) {
		t.Fatalf("doPushCommand() error = %v, want %v", err, wantErr)
	}
	if _, err := os.Stat(exportPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected manifest export file not to exist, stat error = %v", err)
	}

	wantCalls := []string{"OnFileLoading", "OnCopied"}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("handler calls = %v, want %v", got, wantCalls)
	}
}

func Test_doPushCommand_renderError(t *testing.T) {
	ctx := context.Background()
	recorder := &displaytest.CallRecorder{}
	wantErr := stderrors.New("Render failed")
	metadataHandler := &displaytest.RecordingPushMetadataHandler{
		Recorder:  recorder,
		RenderErr: wantErr,
	}

	store, err := file.New("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	memoryStore := memory.New()
	union := contentutil.MultiReadOnlyTarget(memoryStore, store)

	filePath := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatal(err)
	}

	exportPath := filepath.Join(t.TempDir(), "manifest.json")
	opts := &pushOptions{}
	opts.FileRefs = []string{filePath}
	opts.Reference = "tag1"
	opts.PackVersion = oras.PackManifestVersion1_1
	opts.artifactType = "application/example"
	opts.extraRefs = []string{"tag2"}
	opts.ManifestExportPath = exportPath

	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder}
	err = doPushCommand(
		ctx,
		opts,
		oras.PackManifestOptions{},
		store,
		memoryStore,
		union,
		statusHandler,
		metadataHandler,
		func() (oras.GraphTarget, error) {
			return memory.New(), nil
		},
	)
	if !stderrors.Is(err, wantErr) {
		t.Fatalf("doPushCommand() error = %v, want %v", err, wantErr)
	}
	if _, err := os.Stat(exportPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected manifest export file not to exist, stat error = %v", err)
	}

	wantCalls := []string{"OnFileLoading", "OnCopied", "OnTagged:tag2", "Render"}
	got := recorder.Snapshot()
	if !reflect.DeepEqual(got, wantCalls) {
		t.Errorf("handler calls = %v, want %v", got, wantCalls)
	}
}
