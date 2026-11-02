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
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/registry"
	"golang.org/x/sync/errgroup"
	"oras.land/oras/internal/docker"
)

// MediaTypeArtifactManifest specifies the media type for a content descriptor.
const MediaTypeArtifactManifest = "application/vnd.oci.artifact.manifest.v1+json"

// Artifact describes an artifact manifest.
// This structure provides `application/vnd.oci.artifact.manifest.v1+json` mediatype when marshalled to JSON.
//
// This manifest type was introduced in image-spec v1.1.0-rc1 and was removed in
// image-spec v1.1.0-rc3. It is not part of the current image-spec and is kept
// here for Go compatibility.
//
// Reference: https://github.com/opencontainers/image-spec/pull/999
type Artifact struct {
	// MediaType is the media type of the object this schema refers to.
	MediaType string `json:"mediaType"`

	// ArtifactType is the IANA media type of the artifact this schema refers to.
	ArtifactType string `json:"artifactType"`

	// Blobs is a collection of blobs referenced by this manifest.
	Blobs []ocispec.Descriptor `json:"blobs,omitempty"`

	// Subject (reference) is an optional link from the artifact to another manifest forming an association between the artifact and the other manifest.
	Subject *ocispec.Descriptor `json:"subject,omitempty"`

	// Annotations contains arbitrary metadata for the artifact manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Successors returns the nodes directly pointed by the current node, picking
// out subject and config descriptor if applicable.
// Returning nil when no subject and config found.
func Successors(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) (nodes []ocispec.Descriptor, subject, config *ocispec.Descriptor, err error) {
	switch node.MediaType {
	case docker.MediaTypeManifest, ocispec.MediaTypeImageManifest:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var manifest ocispec.Manifest
		if err = json.Unmarshal(fetched, &manifest); err != nil {
			return
		}
		nodes = manifest.Layers
		subject = manifest.Subject
		config = &manifest.Config
	case MediaTypeArtifactManifest:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var manifest Artifact
		if err = json.Unmarshal(fetched, &manifest); err != nil {
			return
		}
		nodes = manifest.Blobs
		subject = manifest.Subject
	case ocispec.MediaTypeImageIndex:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var index ocispec.Index
		if err = json.Unmarshal(fetched, &index); err != nil {
			return
		}
		nodes = index.Manifests
		subject = index.Subject
	default:
		nodes, err = content.Successors(ctx, fetcher, node)
	}
	return
}

// FindPredecessors returns all predecessors of descs in src concurrently.
func FindPredecessors(ctx context.Context, src oras.ReadOnlyGraphTarget, descs []ocispec.Descriptor, opts oras.ExtendedCopyGraphOptions) ([]ocispec.Descriptor, error) {
	var predecessors []ocispec.Descriptor
	g, ctx := errgroup.WithContext(ctx)
	var m sync.Mutex
	if opts.Concurrency != 0 {
		g.SetLimit(opts.Concurrency)
	}
	if opts.FindPredecessors == nil {
		opts.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return src.Predecessors(ctx, desc)
		}
	}
	for _, desc := range descs {
		g.Go(func(node ocispec.Descriptor) func() error {
			return func() error {
				descs, err := opts.FindPredecessors(ctx, src, node)
				if err != nil {
					return err
				}
				m.Lock()
				defer m.Unlock()
				predecessors = append(predecessors, descs...)
				return nil
			}
		}(desc))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return predecessors, nil
}

// RecursiveFindReferrers finds all referrers of the given descriptors recursively.
func RecursiveFindReferrers(ctx context.Context, src oras.ReadOnlyGraphTarget, descs []ocispec.Descriptor, opts oras.ExtendedCopyGraphOptions) ([]ocispec.Descriptor, error) {
	if opts.FindPredecessors == nil {
		opts.FindPredecessors = func(ctx context.Context, src content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return registry.Referrers(ctx, src, desc, "")
		}
	}
	var allReferrers []ocispec.Descriptor
	// visited tracks descriptors that have already been collected so that a
	// cyclic referrer graph (e.g. A -> B -> A), which a malicious registry can
	// craft, does not cause unbounded recursion and memory growth.
	visited := make(map[digest.Digest]bool)
	for len(descs) > 0 {
		referrers, err := FindPredecessors(ctx, src, descs, opts)
		if err != nil {
			return nil, err
		}
		var next []ocispec.Descriptor
		for _, referrer := range referrers {
			if visited[referrer.Digest] {
				continue
			}
			visited[referrer.Digest] = true
			next = append(next, referrer)
		}
		allReferrers = append(allReferrers, next...)
		descs = next
	}
	return allReferrers, nil
}

// FilteredSuccessors fetches successors and returns filtered ones.
func FilteredSuccessors(ctx context.Context, desc ocispec.Descriptor, fetcher content.Fetcher, filter func(ocispec.Descriptor) bool) ([]ocispec.Descriptor, error) {
	allSuccessors, err := content.Successors(ctx, fetcher, desc)
	if err != nil {
		return nil, err
	}

	var successors []ocispec.Descriptor
	for _, s := range allSuccessors {
		if filter(s) {
			successors = append(successors, s)
		}
	}
	return successors, nil
}
