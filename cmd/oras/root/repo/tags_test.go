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

package repo

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
)

func newTagsTestLayout(t *testing.T) *oci.Store {
	t.Helper()
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	desc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"v1", "v2"} {
		if err := store.Tag(ctx, desc, tag); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func Test_doShowTags_handlerCalls(t *testing.T) {
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingRepoTagsHandler{Recorder: recorder}
	if err := doShowTags(context.Background(), newTagsTestLayout(t), &showTagsOptions{}, "", "", handler); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagListed", "OnTagListed", "Render"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doShowTags_handlerErrorSkipsRender(t *testing.T) {
	wantedErr := errors.New("tag listing failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingRepoTagsHandler{Recorder: recorder, TagListedErr: wantedErr}
	if err := doShowTags(context.Background(), newTagsTestLayout(t), &showTagsOptions{}, "", "", handler); !errors.Is(err, wantedErr) {
		t.Fatalf("doShowTags() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagListed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
