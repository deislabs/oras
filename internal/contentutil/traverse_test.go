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

package contentutil

import (
	"context"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/graph"
)

func TestIsManifestMediaType(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{ocispec.MediaTypeImageManifest, true},
		{ocispec.MediaTypeImageIndex, true},
		{docker.MediaTypeManifest, true},
		{docker.MediaTypeManifestList, true},
		{graph.MediaTypeArtifactManifest, true},
		{ocispec.MediaTypeImageLayer, false},
		{ocispec.MediaTypeImageLayerGzip, false},
		{"application/vnd.oci.image.config.v1+json", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsManifestMediaType(tt.mediaType); got != tt.want {
			t.Errorf("IsManifestMediaType(%q) = %v, want %v", tt.mediaType, got, tt.want)
		}
	}
}

// fakeGraphTarget is a minimal oras.GraphTarget used to observe Exists calls.
type fakeGraphTarget struct {
	oras.GraphTarget
	existsCalled bool
	existsResult bool
	existsErr    error
}

func (f *fakeGraphTarget) Exists(_ context.Context, _ ocispec.Descriptor) (bool, error) {
	f.existsCalled = true
	return f.existsResult, f.existsErr
}

func TestTraversingTarget_Exists(t *testing.T) {
	tests := []struct {
		name             string
		mediaType        string
		underlyingExists bool
		underlyingErr    error
		wantExists       bool
		wantErr          bool
		wantDelegate     bool
	}{
		{
			name:         "image manifest is reported as missing without delegating",
			mediaType:    ocispec.MediaTypeImageManifest,
			wantDelegate: false,
		},
		{
			name:         "image index is reported as missing without delegating",
			mediaType:    ocispec.MediaTypeImageIndex,
			wantDelegate: false,
		},
		{
			name:         "docker manifest list is reported as missing without delegating",
			mediaType:    docker.MediaTypeManifestList,
			wantDelegate: false,
		},
		{
			name:             "blob existence delegates and propagates true",
			mediaType:        ocispec.MediaTypeImageLayer,
			underlyingExists: true,
			wantExists:       true,
			wantDelegate:     true,
		},
		{
			name:             "blob existence delegates and propagates false",
			mediaType:        ocispec.MediaTypeImageLayerGzip,
			underlyingExists: false,
			wantExists:       false,
			wantDelegate:     true,
		},
		{
			name:          "blob existence delegates and propagates error",
			mediaType:     ocispec.MediaTypeImageLayer,
			underlyingErr: errors.New("boom"),
			wantErr:       true,
			wantDelegate:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeGraphTarget{existsResult: tt.underlyingExists, existsErr: tt.underlyingErr}
			v := &TraversingTarget{GraphTarget: f}
			got, err := v.Exists(context.Background(), ocispec.Descriptor{
				MediaType: tt.mediaType,
				Digest:    digest.FromString("x"),
				Size:      1,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Exists() err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantExists {
				t.Errorf("Exists() = %v, want %v", got, tt.wantExists)
			}
			if f.existsCalled != tt.wantDelegate {
				t.Errorf("delegate Exists called = %v, want %v", f.existsCalled, tt.wantDelegate)
			}
		})
	}
}
