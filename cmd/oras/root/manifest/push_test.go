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

package manifest

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/option"
)

func Test_pushManifestCmd_invalidExtraTag(t *testing.T) {
	cmd := pushCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1,,invalid@@tag",
		"manifest.json",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != "empty extra tag" {
		t.Fatalf("got %v, want empty extra tag error", err)
	}
}

func Test_pushManifestCmd_invalidHeader(t *testing.T) {
	cmd := pushCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1",
		"manifest.json",
		"--header", "invalid",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != `invalid header: "invalid"` {
		t.Fatalf("got %v, want invalid header error", err)
	}
}

func TestPushManifestToTarget_handlerCallOrderAndError(t *testing.T) {
	ctx := context.Background()
	target := memory.New()
	wantedErr := errors.New("manifest push failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{
		Recorder:        recorder,
		ManifestPushErr: wantedErr,
	}

	err := pushManifestToTarget(ctx, target, "application/vnd.oci.image.manifest.v1+json", []byte(`{"schemaVersion":2}`), "v1", false, statusHandler)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("pushManifestToTarget() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestPushing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status calls = %v, want %v", got, want)
	}

	statusHandler.ManifestPushErr = nil
	if err := pushManifestToTarget(ctx, target, "application/vnd.oci.image.manifest.v1+json", []byte(`{"schemaVersion":2}`), "v1", true, statusHandler); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestPushing", "OnManifestPushSkipped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status calls = %v, want %v", got, want)
	}

	if err := pushManifestToTarget(ctx, target, "application/vnd.oci.image.manifest.v1+json", []byte(`{"schemaVersion":2}`), "v2", false, statusHandler); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestPushing", "OnManifestPushSkipped", "OnManifestPushing", "OnManifestPushed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status calls = %v, want %v", got, want)
	}
}

type failingManifestPushTarget struct {
	*memory.Store
	err error
}

func (t *failingManifestPushTarget) Push(_ context.Context, _ ocispec.Descriptor, _ io.Reader) error {
	return t.err
}

func TestPushManifestToTarget_targetError(t *testing.T) {
	wantedErr := errors.New("target push failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder}
	target := &failingManifestPushTarget{Store: memory.New(), err: wantedErr}

	err := pushManifestToTarget(context.Background(), target, "application/vnd.oci.image.manifest.v1+json", []byte(`{"schemaVersion":2}`), "v1", false, statusHandler)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("pushManifestToTarget() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestPushing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status calls = %v, want %v", got, want)
	}
}

func Test_doPushManifest_handlerCalls(t *testing.T) {
	contentBytes := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json"}`)
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, contentBytes)
	opts := pushOptions{Target: option.Target{Reference: "v1"}, extraRefs: []string{"v2"}}
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingPushMetadataHandler{Recorder: recorder}

	if err := doPushManifest(context.Background(), opts, memory.New(), desc.MediaType, contentBytes, desc, statusHandler, metadataHandler); err != nil {
		t.Fatal(err)
	}

	want := []string{"OnManifestPushing", "OnManifestPushed", "OnManifestPushed", "OnTagged:v2", "Render"}
	if got := recorder.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doPushManifest_statusErrorStopsPush(t *testing.T) {
	contentBytes := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json"}`)
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, contentBytes)
	opts := pushOptions{Target: option.Target{Reference: "v1"}}
	wantedErr := errors.New("status failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingPushStatusHandler{Recorder: recorder, ManifestPushErr: wantedErr}
	metadataHandler := &displaytest.RecordingPushMetadataHandler{Recorder: recorder}

	err := doPushManifest(context.Background(), opts, memory.New(), desc.MediaType, contentBytes, desc, statusHandler, metadataHandler)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("doPushManifest() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestPushing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
