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

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/testutils"
)

func TestSuccessors(t *testing.T) {
	mockFetcher := testutils.NewMockFetcher()
	fetcher := mockFetcher.Fetcher
	ctx := context.Background()
	type args struct {
		ctx     context.Context
		fetcher content.Fetcher
		node    ocispec.Descriptor
	}
	tests := []struct {
		name        string
		args        args
		wantNodes   []ocispec.Descriptor
		wantSubject *ocispec.Descriptor
		wantConfig  *ocispec.Descriptor
		wantErr     bool
	}{
		{"should failed to get non-existent OCI image", args{ctx, fetcher, ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest}}, nil, nil, nil, true},
		{"should failed to get non-existent docker image", args{ctx, fetcher, ocispec.Descriptor{MediaType: docker.MediaTypeManifest}}, nil, nil, nil, true},
		{"should get successors of a docker image", args{ctx, fetcher, mockFetcher.DockerImage}, nil, nil, &mockFetcher.Config, false},
		{"should get successors of an OCI image", args{ctx, fetcher, mockFetcher.OciImage}, []ocispec.Descriptor{mockFetcher.ImageLayer}, &mockFetcher.Subject, &mockFetcher.Config, false},
		{"should get successors of an index", args{ctx, fetcher, mockFetcher.Index}, []ocispec.Descriptor{mockFetcher.Subject}, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodes, gotSubject, gotConfig, err := Successors(tt.args.ctx, tt.args.fetcher, tt.args.node)
			if (err != nil) != tt.wantErr {
				t.Errorf("Successors() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotNodes, tt.wantNodes) {
				t.Errorf("Successors() gotNodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			if !reflect.DeepEqual(gotSubject, tt.wantSubject) {
				t.Errorf("Successors() gotSubject = %v, want %v", gotSubject, tt.wantSubject)
			}
			if !reflect.DeepEqual(gotConfig, tt.wantConfig) {
				t.Errorf("Successors() gotConfig = %v, want %v", gotConfig, tt.wantConfig)
			}
		})
	}
}

func TestDescriptor_GetSuccessors(t *testing.T) {
	mockFetcher := testutils.NewMockFetcher()

	allFilter := func(ocispec.Descriptor) bool {
		return true
	}
	got, err := FilteredSuccessors(context.Background(), mockFetcher.OciImage, mockFetcher.Fetcher, allFilter)
	if nil != err {
		t.Errorf("FilteredSuccessors unexpected error %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Expected 2 successors got %v", len(got))
	}
	if mockFetcher.Subject.Digest != got[0].Digest {
		t.Errorf("FilteredSuccessors got %v, want %v", got[0], mockFetcher.Subject)
	}
	if mockFetcher.Config.Digest != got[1].Digest {
		t.Errorf("FilteredSuccessors got %v, want %v", got[1], mockFetcher.Subject)
	}

	noConfig := func(desc ocispec.Descriptor) bool {
		return desc.Digest != mockFetcher.Config.Digest
	}
	got, err = FilteredSuccessors(context.Background(), mockFetcher.OciImage, mockFetcher.Fetcher, noConfig)
	if nil != err {
		t.Errorf("FilteredSuccessors unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Expected 1 successors got %v", len(got))
	}
	if mockFetcher.Subject.Digest != got[0].Digest {
		t.Errorf("FilteredSuccessors got %v, want %v", got[0], mockFetcher.Subject)
	}

	got, err = FilteredSuccessors(context.Background(), ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest}, mockFetcher.Fetcher, allFilter)
	if nil == err {
		t.Error("FilteredSuccessors expected error")
	}
	if got != nil {
		t.Errorf("FilteredSuccessors unexpected %v", got)
	}
}

func TestFindPredecessors(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	// prepare subjects
	var subjects []ocispec.Descriptor
	for i := range 2 {
		subject, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "test/subject", oras.PackManifestOptions{
			ManifestAnnotations: map[string]string{
				"test.subject.number": strconv.Itoa(i),
			},
		})
		if err != nil {
			t.Fatalf("PackManifest unexpected error: %v", err)
		}
		subjects = append(subjects, subject)
	}

	// prepare referrers
	referrers := make(map[digest.Digest]ocispec.Descriptor)
	for _, subject := range subjects {
		for i := range 3 {
			referrer, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "test/referrer", oras.PackManifestOptions{
				Subject: &subject,
				ManifestAnnotations: map[string]string{
					"test.referrer.number": strconv.Itoa(i),
				},
			})
			if err != nil {
				t.Fatalf("PackManifest unexpected error: %v", err)
			}
			referrers[referrer.Digest] = referrer
		}
	}

	// test FindPredecessors
	opts := oras.DefaultExtendedCopyGraphOptions
	gotReferrers, err := FindPredecessors(ctx, store, subjects, opts)
	if err != nil {
		t.Fatalf("FindPredecessors unexpected error: %v", err)
	}
	if len(gotReferrers) != len(referrers) {
		t.Fatalf("FindPredecessors got %d referrers, want %d", len(gotReferrers), len(referrers))
	}
	for _, got := range gotReferrers {
		wantReferrer, ok := referrers[got.Digest]
		if !ok {
			t.Errorf("FindPredecessors got unexpected referrer %v", got)
		}
		if !reflect.DeepEqual(got, wantReferrer) {
			t.Errorf("FindPredecessors got referrer %v, want %v", got, wantReferrer)
		}
	}
}

func TestRecursiveFindReferrers(t *testing.T) {
	// prepare test data
	ctx := context.Background()
	target := memory.New()

	// create manifests
	manifestDesc1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/manifest1", oras.PackManifestOptions{})
	if err != nil {
		t.Fatalf("failed to create manifest descriptor 1: %v", err)
	}
	manifestDesc2, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/manifest2", oras.PackManifestOptions{})
	if err != nil {
		t.Fatalf("failed to create manifest descriptor 2: %v", err)
	}
	// create flatten referrers
	referrerDesc1_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer1_1", oras.PackManifestOptions{Subject: &manifestDesc1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 1: %v", err)
	}
	referrerDesc1_2, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer1_2", oras.PackManifestOptions{Subject: &manifestDesc1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 1: %v", err)
	}
	// create nested referrers
	referrerDesc2_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer2_1", oras.PackManifestOptions{Subject: &manifestDesc2})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 2: %v", err)
	}
	referrerDesc2_1_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer2_1_1", oras.PackManifestOptions{Subject: &referrerDesc2_1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 2: %v", err)
	}
	// create index manifest
	index := ocispec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Manifests: []ocispec.Descriptor{
			manifestDesc1,
			manifestDesc2,
		},
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("failed to marshal index manifest: %v", err)
	}
	indexDesc, err := oras.PushBytes(ctx, target, ocispec.MediaTypeImageIndex, indexBytes)
	if err != nil {
		t.Fatalf("failed to push index manifest: %v", err)
	}
	// add nested referrers to the index
	referrerDesc3_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/index", oras.PackManifestOptions{Subject: &indexDesc})
	if err != nil {
		t.Fatalf("failed to create index referrer: %v", err)
	}
	referrerDesc3_1_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/index_referrer1", oras.PackManifestOptions{Subject: &referrerDesc3_1})
	if err != nil {
		t.Fatalf("failed to create nested index referrer: %v", err)
	}

	t.Run("find referers for empty manifest list", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		if len(gotReferrers) != 0 {
			t.Errorf("RecursiveFindReferrers got %d referrers, want 0", len(gotReferrers))
		}
	})

	t.Run("cyclic referrer graph terminates", func(t *testing.T) {
		// A content-addressable store cannot hold a real cycle, so inject a
		// FindPredecessors that reports a cyclic referrer graph A -> B -> A,
		// as a malicious registry could. Without cycle detection this loops
		// forever; with it the traversal terminates and each node is collected
		// exactly once.
		descA := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("A"), Size: 1}
		descB := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("B"), Size: 1}
		opts := oras.DefaultExtendedCopyGraphOptions
		opts.FindPredecessors = func(_ context.Context, _ content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			switch desc.Digest {
			case descA.Digest:
				return []ocispec.Descriptor{descB}, nil
			case descB.Digest:
				return []ocispec.Descriptor{descA}, nil
			default:
				return nil, nil
			}
		}
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{descA}, opts)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		wantReferrers := map[digest.Digest]ocispec.Descriptor{
			descA.Digest: descA,
			descB.Digest: descB,
		}
		if len(gotReferrers) != len(wantReferrers) {
			t.Fatalf("RecursiveFindReferrers got %d referrers, want %d", len(gotReferrers), len(wantReferrers))
		}
		for _, got := range gotReferrers {
			if _, ok := wantReferrers[got.Digest]; !ok {
				t.Errorf("RecursiveFindReferrers got unexpected referrer %v", got)
			}
		}
	})

	t.Run("find referrers for manifest without referrers", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{referrerDesc3_1_1}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		if len(gotReferrers) != 0 {
			t.Errorf("RecursiveFindReferrers got %d referrers, want 0", len(gotReferrers))
		}
	})

	t.Run("find referrers for manifest 1", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{manifestDesc1}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		wantReferrers := map[digest.Digest]ocispec.Descriptor{
			referrerDesc1_1.Digest: referrerDesc1_1,
			referrerDesc1_2.Digest: referrerDesc1_2,
		}
		if len(gotReferrers) != len(wantReferrers) {
			t.Fatalf("RecursiveFindReferrers got %d referrers, want %d", len(gotReferrers), len(wantReferrers))
		}
		for _, got := range gotReferrers {
			want, ok := wantReferrers[got.Digest]
			if !ok {
				t.Errorf("RecursiveFindReferrers got unexpected referrer %v", got)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RecursiveFindReferrers got referrer %v, want %v", got, want)
			}
		}
	})

	t.Run("find referrers for manifest 2", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{manifestDesc2}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		wantReferrers := map[digest.Digest]ocispec.Descriptor{
			referrerDesc2_1.Digest:   referrerDesc2_1,
			referrerDesc2_1_1.Digest: referrerDesc2_1_1,
		}
		if len(gotReferrers) != len(wantReferrers) {
			t.Fatalf("RecursiveFindReferrers got %d referrers, want %d", len(gotReferrers), len(wantReferrers))
		}
		for _, got := range gotReferrers {
			want, ok := wantReferrers[got.Digest]
			if !ok {
				t.Errorf("RecursiveFindReferrers got unexpected referrer %v", got)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RecursiveFindReferrers got referrer %v, want %v", got, want)
			}
		}
	})

	t.Run("find referrers for manifest 1 and 2", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{manifestDesc1, manifestDesc2}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		wantReferrers := map[digest.Digest]ocispec.Descriptor{
			referrerDesc1_1.Digest:   referrerDesc1_1,
			referrerDesc1_2.Digest:   referrerDesc1_2,
			referrerDesc2_1.Digest:   referrerDesc2_1,
			referrerDesc2_1_1.Digest: referrerDesc2_1_1,
		}
		if len(gotReferrers) != len(wantReferrers) {
			t.Fatalf("RecursiveFindReferrers got %d referrers, want %d", len(gotReferrers), len(wantReferrers))
		}
		for _, got := range gotReferrers {
			want, ok := wantReferrers[got.Digest]
			if !ok {
				t.Errorf("RecursiveFindReferrers got unexpected referrer %v", got)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RecursiveFindReferrers got referrer %v, want %v", got, want)
			}
		}
	})

	t.Run("find referrers for index manifest", func(t *testing.T) {
		gotReferrers, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{indexDesc}, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("RecursiveFindReferrers unexpected error: %v", err)
		}
		wantReferrers := map[digest.Digest]ocispec.Descriptor{
			referrerDesc3_1.Digest:   referrerDesc3_1,
			referrerDesc3_1_1.Digest: referrerDesc3_1_1,
		}
		if len(gotReferrers) != len(wantReferrers) {
			t.Fatalf("RecursiveFindReferrers got %d referrers, want %d", len(gotReferrers), len(wantReferrers))
		}
		for _, got := range gotReferrers {
			want, ok := wantReferrers[got.Digest]
			if !ok {
				t.Errorf("RecursiveFindReferrers got unexpected referrer %v", got)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RecursiveFindReferrers got referrer %v, want %v", got, want)
			}
		}
	})

	t.Run("bad FindPredecessors options", func(t *testing.T) {
		testErr := errors.New("test error")
		opts := oras.ExtendedCopyGraphOptions{
			FindPredecessors: func(_ context.Context, _ content.ReadOnlyGraphStorage, _ ocispec.Descriptor) ([]ocispec.Descriptor, error) {
				return nil, testErr
			},
		}
		_, err := RecursiveFindReferrers(ctx, target, []ocispec.Descriptor{manifestDesc1}, opts)
		if !errors.Is(err, testErr) {
			t.Errorf("expected error %v, got %v", testErr, err)
		}
	})
}
