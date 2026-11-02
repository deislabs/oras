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
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/internal/listener"
)

func Test_tagManifest_remoteNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	uri, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := tagCmd()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--plain-http", uri.Host + "/test:v1", "v2"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("tag error = nil, want not found error")
	}
}

func Test_tagCmd_requiresSourceAndDestination(t *testing.T) {
	rootCmd := &cobra.Command{Use: "oras"}
	rootCmd.AddCommand(tagCmd())
	rootCmd.SetArgs([]string{"tag", "localhost:5000/hello:v1"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected an argument validation error")
	}
	if !strings.Contains(err.Error(), "at least 2 arguments") {
		t.Fatalf("expected an argument count error, got %v", err)
	}
}

func TestTagListener_handlerCallOrderAndError(t *testing.T) {
	target := memory.New()
	desc := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest}
	wantedErr := errors.New("tagging failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingTagHandler{Recorder: recorder, TaggingErr: wantedErr}
	wrapped := listener.NewTagListener(target, handler.OnTagging, handler.OnTagged)
	if err := wrapped.Tag(context.Background(), desc, "v1"); !errors.Is(err, wantedErr) {
		t.Fatalf("Tag() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagging"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doTag_handlerCallOrderAndError(t *testing.T) {
	ctx := context.Background()
	target := memory.New()
	desc, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Tag(ctx, desc, "source"); err != nil {
		t.Fatal(err)
	}
	wantedErr := errors.New("tagging failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingTagHandler{Recorder: recorder, TaggingErr: wantedErr}
	opts := oras.DefaultTagNOptions
	err = doTag(ctx, target, "source", []string{"destination"}, opts, handler)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("doTag() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagging"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doTag_handlerCallOrder(t *testing.T) {
	ctx := context.Background()
	target := memory.New()
	desc, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Tag(ctx, desc, "source"); err != nil {
		t.Fatal(err)
	}
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingTagHandler{Recorder: recorder}
	err = doTag(ctx, target, "source", []string{"destination"}, oras.DefaultTagNOptions, handler)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagging", "OnTagged"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
