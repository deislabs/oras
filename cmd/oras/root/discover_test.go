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
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
)

// cyclicReferrerTarget is a minimal ReadOnlyGraphTarget that serves a
// configurable referrer graph via the Referrers API. It lets tests model a
// malicious registry that returns a cyclic referrer graph.
type cyclicReferrerTarget struct {
	referrers map[digest.Digest][]ocispec.Descriptor
}

func (t *cyclicReferrerTarget) Referrers(_ context.Context, desc ocispec.Descriptor, _ string, fn func(referrers []ocispec.Descriptor) error) error {
	return fn(t.referrers[desc.Digest])
}

func (t *cyclicReferrerTarget) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (t *cyclicReferrerTarget) Exists(context.Context, ocispec.Descriptor) (bool, error) {
	return false, nil
}

func (t *cyclicReferrerTarget) Predecessors(context.Context, ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return nil, nil
}

func (t *cyclicReferrerTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, errors.New("not implemented")
}

func TestFetchAllReferrers_CyclicGraphTerminates(t *testing.T) {
	descA := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("A"), Size: 1}
	descB := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("B"), Size: 1}
	// A -> B -> A, a cycle a malicious registry could craft.
	target := &cyclicReferrerTarget{referrers: map[digest.Digest][]ocispec.Descriptor{
		descA.Digest: {descB},
		descB.Digest: {descA},
	}}
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingDiscoverHandler{Recorder: recorder}

	// depth 0 means unlimited; without cycle detection this never returns.
	if err := fetchAllReferrers(context.Background(), target, descA, "", handler, 0, make(map[digest.Digest]bool)); err != nil {
		t.Fatalf("fetchAllReferrers() error = %v", err)
	}
	// Each edge (A->B and B->A) is reported exactly once.
	calls := recorder.Snapshot()
	if got, want := calls, []string{"OnDiscovered", "OnDiscovered"}; !reflect.DeepEqual(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func TestFetchAllReferrers_HandlerErrorStopsTraversal(t *testing.T) {
	wantedErr := errors.New("discover handler failed")
	desc := ocispec.Descriptor{Digest: digest.FromString("root"), MediaType: ocispec.MediaTypeImageManifest}
	target := &cyclicReferrerTarget{referrers: map[digest.Digest][]ocispec.Descriptor{
		desc.Digest: {{Digest: digest.FromString("child"), MediaType: ocispec.MediaTypeImageManifest}},
	}}
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingDiscoverHandler{Recorder: recorder, Err: wantedErr}
	if err := fetchAllReferrers(context.Background(), target, desc, "", handler, 0, make(map[digest.Digest]bool)); !errors.Is(err, wantedErr) {
		t.Fatalf("fetchAllReferrers() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnDiscovered"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func TestDiscoverCmd_OutputDeprecationMessage(t *testing.T) {
	for _, format := range []string{"tree", "table", "json"} {
		t.Run(format, func(t *testing.T) {
			cmd := discoverCmd()
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)
			if err := cmd.ParseFlags([]string{"--output", format}); err != nil {
				t.Fatal(err)
			}
			if err := cmd.PreRunE(cmd, []string{"localhost:5000/hello:v1"}); err != nil {
				t.Fatal(err)
			}
			want := "[DEPRECATED] --output is deprecated, try `--format " + format + "` instead"
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
			}
		})
	}
}

func Test_doDiscover_handlerCalls(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	subject, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example.referrer", oras.PackManifestOptions{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingDiscoverHandler{Recorder: recorder}

	if err := doDiscover(ctx, store, subject, "", 0, handler); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnDiscovered", "Render"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doDiscover_handlerErrorSkipsRender(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	subject, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example.referrer", oras.PackManifestOptions{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("discover handler failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingDiscoverHandler{Recorder: recorder, Err: wantErr}

	if err := doDiscover(ctx, store, subject, "", 0, handler); !errors.Is(err, wantErr) {
		t.Fatalf("doDiscover() error = %v, want %v", err, wantErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnDiscovered"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
