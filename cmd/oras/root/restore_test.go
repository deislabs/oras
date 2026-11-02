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
	"slices"
	"testing"
	"time"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/content/oci"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	orasio "oras.land/oras/internal/io"
)

func newRestoreTestTar(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	layoutDir := t.TempDir()
	layout, err := oci.New(layoutDir)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := oras.PackManifest(ctx, layout, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Tag(ctx, desc, "v1"); err != nil {
		t.Fatal(err)
	}
	tarPath := filepath.Join(t.TempDir(), "backup.tar")
	tarFile, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tarFile.Close() }()
	if err := orasio.TarDirectory(tarFile, layoutDir); err != nil {
		t.Fatal(err)
	}
	return tarPath
}

func Test_doRestore_handlerCalls(t *testing.T) {
	opts := &restoreOptions{input: newRestoreTestTar(t), repository: "example"}
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingRestoreStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingRestoreMetadataHandler{Recorder: recorder}

	if err := doRestore(context.Background(), opts, memory.New(), time.Now(), statusHandler, metadataHandler); err != nil {
		t.Fatal(err)
	}

	calls := recorder.Snapshot()
	if len(calls) < 6 {
		t.Fatalf("handler calls = %v, want at least 6", calls)
	}
	if got, want := calls[:3], []string{"OnTarLoaded", "OnTagsFound", "StartTracking"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first handler calls = %v, want %v", got, want)
	}
	if got, want := calls[len(calls)-3:], []string{"StopTracking", "OnArtifactPushed", "OnRestoreCompleted"}; !reflect.DeepEqual(got, want) {
		t.Errorf("last handler calls = %v, want %v", got, want)
	}
}

func Test_doRestore_dryRun(t *testing.T) {
	opts := &restoreOptions{input: newRestoreTestTar(t), repository: "example", dryRun: true}
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingRestoreStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingRestoreMetadataHandler{Recorder: recorder}

	if err := doRestore(context.Background(), opts, memory.New(), time.Now(), statusHandler, metadataHandler); err != nil {
		t.Fatal(err)
	}

	want := []string{"OnTarLoaded", "OnTagsFound", "OnArtifactPushed", "OnRestoreCompleted"}
	if got := recorder.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_doRestore_artifactPushedError(t *testing.T) {
	opts := &restoreOptions{input: newRestoreTestTar(t), repository: "example"}
	wantErr := errors.New("artifact pushed failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingRestoreStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingRestoreMetadataHandler{Recorder: recorder, ArtifactPushedErr: wantErr}

	err := doRestore(context.Background(), opts, memory.New(), time.Now(), statusHandler, metadataHandler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("doRestore() error = %v, want %v", err, wantErr)
	}
	if calls := recorder.Snapshot(); slices.Contains(calls, "OnRestoreCompleted") {
		t.Errorf("handler calls = %v, want no OnRestoreCompleted after an error", calls)
	}
}
