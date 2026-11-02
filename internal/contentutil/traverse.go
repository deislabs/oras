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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/internal/docker"
	"oras.land/oras/internal/graph"
)

// IsManifestMediaType reports whether the given media type denotes a manifest.
func IsManifestMediaType(mediaType string) bool {
	switch mediaType {
	case ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex,
		docker.MediaTypeManifest,
		docker.MediaTypeManifestList,
		graph.MediaTypeArtifactManifest:
		return true
	}
	return false
}

// TraversingTarget wraps an [oras.GraphTarget] and reports manifest
// descriptors as not yet present in the destination. This defeats the
// sub-DAG skip in oras-go's copyGraph (where any descriptor reported as
// existing causes its entire successor tree to be skipped) and forces
// recursive traversal so that missing referenced manifests or blobs are
// discovered and copied.
//
// Non-manifest existence checks still delegate to the wrapped target, so
// content that is already in the destination is not re-uploaded.
type TraversingTarget struct {
	oras.GraphTarget
}

// Exists returns false for manifest descriptors regardless of the
// underlying target's state; for non-manifest descriptors it delegates
// to the wrapped target.
func (v *TraversingTarget) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	if IsManifestMediaType(target.MediaType) {
		return false, nil
	}
	return v.GraphTarget.Exists(ctx, target)
}
