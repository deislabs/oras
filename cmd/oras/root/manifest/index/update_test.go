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

package index

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/cmd/oras/internal/display/status"
	"oras.land/oras/internal/docker"
)

type testUpdateDisplayStatus struct {
	onFetchingError        bool
	onFetchedError         bool
	onIndexPackedError     bool
	onIndexPushedError     bool
	onManifestRemovedError bool
	onManifestAddedError   bool
	onIndexMergedError     bool
}

func (tds *testUpdateDisplayStatus) OnFetching(_ string) error {
	if tds.onFetchingError {
		return fmt.Errorf("OnFetching error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnFetched(_ string, _ ocispec.Descriptor) error {
	if tds.onFetchedError {
		return fmt.Errorf("OnFetched error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnIndexPacked(_ ocispec.Descriptor) error {
	if tds.onIndexPackedError {
		return fmt.Errorf("error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnIndexPushed(_ string) error {
	if tds.onIndexPushedError {
		return fmt.Errorf("error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnManifestRemoved(_ digest.Digest) error {
	if tds.onManifestRemovedError {
		return fmt.Errorf("OnManifestRemoved error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnManifestAdded(_ string, _ ocispec.Descriptor) error {
	if tds.onManifestAddedError {
		return fmt.Errorf("error")
	}
	return nil
}

func (tds *testUpdateDisplayStatus) OnIndexMerged(_ string, _ ocispec.Descriptor) error {
	if tds.onIndexMergedError {
		return fmt.Errorf("error")
	}
	return nil
}

var (
	A = ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Size:      16,
		Digest:    "sha256:58efe73e78fe043ca31b89007a025c594ce12aa7e6da27d21c7b14b50112e255",
	}
	B = ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Size:      18,
		Digest:    "sha256:9d16f5505246424aed7116cb21216704ba8c919997d0f1f37e154c11d509e1d2",
	}
	C = ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Size:      19,
		Digest:    "sha256:fd6ed2f36b5465244d5dc86cb4e7df0ab8a9d24adc57825099f522fe009a22bb",
	}
)

func Test_doRemoveManifests(t *testing.T) {
	tests := []struct {
		name          string
		manifests     []ocispec.Descriptor
		digestSet     map[digest.Digest]bool
		displayStatus status.ManifestIndexUpdateHandler
		indexRef      string
		want          []ocispec.Descriptor
		wantErr       bool
	}{
		{
			name:          "remove one matched item",
			manifests:     []ocispec.Descriptor{A, B, C},
			digestSet:     map[digest.Digest]bool{B.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test01",
			want:          []ocispec.Descriptor{A, C},
			wantErr:       false,
		},
		{
			name:          "remove all matched items",
			manifests:     []ocispec.Descriptor{A, B, A, C, A, A, A},
			digestSet:     map[digest.Digest]bool{A.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test02",
			want:          []ocispec.Descriptor{B, C},
			wantErr:       false,
		},
		{
			name:          "remove correctly when there is only one item",
			manifests:     []ocispec.Descriptor{A},
			digestSet:     map[digest.Digest]bool{A.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test03",
			want:          []ocispec.Descriptor{},
			wantErr:       false,
		},
		{
			name:          "remove multiple distinct manifests",
			manifests:     []ocispec.Descriptor{A, B, C},
			digestSet:     map[digest.Digest]bool{A.Digest: false, C.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test04",
			want:          []ocispec.Descriptor{B},
			wantErr:       false,
		},
		{
			name:          "remove multiple duplicate manifests",
			manifests:     []ocispec.Descriptor{A, B, C, C, B, A, B},
			digestSet:     map[digest.Digest]bool{A.Digest: false, C.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test05",
			want:          []ocispec.Descriptor{B, B, B},
			wantErr:       false,
		},
		{
			name:          "return error when deleting a nonexistent item",
			manifests:     []ocispec.Descriptor{A, C},
			digestSet:     map[digest.Digest]bool{B.Digest: false},
			displayStatus: &testUpdateDisplayStatus{},
			indexRef:      "test06",
			want:          nil,
			wantErr:       true,
		},
		{
			name:          "handler error",
			manifests:     []ocispec.Descriptor{A, B, C},
			digestSet:     map[digest.Digest]bool{B.Digest: false},
			displayStatus: &testUpdateDisplayStatus{onManifestRemovedError: true},
			indexRef:      "test07",
			want:          nil,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := doRemoveManifests(tt.manifests, tt.digestSet, tt.displayStatus, tt.indexRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("removeManifestsFromIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeManifestsFromIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_fetchIndex(t *testing.T) {
	testContext := context.Background()
	tests := []struct {
		name      string
		ctx       context.Context
		handler   status.ManifestIndexUpdateHandler
		target    oras.ReadOnlyTarget
		reference string
		want      ocispec.Index
		wantErr   bool
	}{
		{
			name:      "OnFetching error",
			ctx:       testContext,
			handler:   &testUpdateDisplayStatus{onFetchingError: true},
			target:    NewTestReadOnlyTarget("index"),
			reference: "test",
			want:      ocispec.Index{},
			wantErr:   true,
		},
		{
			name:      "OnFetched error",
			ctx:       testContext,
			handler:   &testUpdateDisplayStatus{onFetchedError: true},
			target:    NewTestReadOnlyTarget("index"),
			reference: "test",
			want:      ocispec.Index{},
			wantErr:   true,
		},
		{
			name:      "Unmarshall error",
			ctx:       testContext,
			handler:   &testUpdateDisplayStatus{},
			target:    NewTestReadOnlyTarget("index"),
			reference: "test",
			want:      ocispec.Index{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fetchIndex(tt.ctx, tt.handler, tt.target, tt.reference)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mergeIndexes(t *testing.T) {
	testContext := context.Background()
	tests := []struct {
		name           string
		ctx            context.Context
		displayStatus  status.ManifestIndexUpdateHandler
		manifests      []ocispec.Descriptor
		target         oras.ReadOnlyTarget
		mergeArguments []string
		want           []ocispec.Descriptor
		wantErr        bool
	}{
		{
			name:           "OnFetching error",
			ctx:            testContext,
			displayStatus:  &testUpdateDisplayStatus{onFetchingError: true},
			manifests:      []ocispec.Descriptor{},
			target:         NewTestReadOnlyTarget("index"),
			mergeArguments: []string{"test"},
			want:           nil,
			wantErr:        true,
		},
		{
			name:           "OnFetched error",
			ctx:            testContext,
			displayStatus:  &testUpdateDisplayStatus{onFetchedError: true},
			manifests:      []ocispec.Descriptor{},
			target:         NewTestReadOnlyTarget("index"),
			mergeArguments: []string{"test"},
			want:           nil,
			wantErr:        true,
		},
		{
			name:           "unmarshall error",
			ctx:            testContext,
			displayStatus:  &testUpdateDisplayStatus{},
			manifests:      []ocispec.Descriptor{},
			target:         NewTestReadOnlyTarget("index"),
			mergeArguments: []string{"test"},
			want:           nil,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeIndexes(tt.ctx, tt.displayStatus, tt.manifests, tt.target, tt.mergeArguments)
			if (err != nil) != tt.wantErr {
				t.Errorf("mergeIndexes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeIndexes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_updateCmd_invalidExtraTag(t *testing.T) {
	cmd := updateCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1",
		"--add", "linux-amd64",
		"--tag", "",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != "empty extra tag" {
		t.Fatalf("got %v, want empty extra tag error", err)
	}
}

type testIndexTarget struct {
	content   []byte
	mediaType string
}

func (t *testIndexTarget) Exists(_ context.Context, _ ocispec.Descriptor) (bool, error) {
	return true, nil
}

func (t *testIndexTarget) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(t.content)), nil
}

func (t *testIndexTarget) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{
		MediaType: t.mediaType,
		Digest:    digest.FromBytes(t.content),
		Size:      int64(len(t.content)),
	}, nil
}

func Test_fetchIndex_mediaType(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		descMediaType string
		want          string
	}{
		{
			name:          "body carries mediaType",
			body:          `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`,
			descMediaType: ocispec.MediaTypeImageIndex,
			want:          ocispec.MediaTypeImageIndex,
		},
		{
			name:          "body omits mediaType",
			body:          `{"schemaVersion":2,"manifests":[]}`,
			descMediaType: ocispec.MediaTypeImageIndex,
			want:          ocispec.MediaTypeImageIndex,
		},
		{
			name:          "docker manifest list is preserved",
			body:          `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json","manifests":[]}`,
			descMediaType: docker.MediaTypeManifestList,
			want:          docker.MediaTypeManifestList,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &testIndexTarget{content: []byte(tt.body), mediaType: tt.descMediaType}
			got, err := fetchIndex(context.Background(), &testUpdateDisplayStatus{}, target, "test")
			if err != nil {
				t.Fatalf("fetchIndex() error = %v", err)
			}
			if got.MediaType != tt.want {
				t.Errorf("fetchIndex() MediaType = %q, want %q", got.MediaType, tt.want)
			}
		})
	}
}

func Test_updateCmd_invalidHeader(t *testing.T) {
	cmd := updateCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1",
		"--add", "linux-amd64",
		"--header", "invalid",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != `invalid header: "invalid"` {
		t.Fatalf("got %v, want invalid header error", err)
	}
}
