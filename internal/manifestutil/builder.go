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

// Package manifestutil provides utilities for building OCI manifests and indexes.
package manifestutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras/internal/descriptor"
	"oras.land/oras/internal/dir"
)

// BuilderOptions configures the manifest builder behavior.
type BuilderOptions struct {
	// MaxBlobsPerManifest limits the number of layers per manifest.
	// If a directory has more files, they are split across multiple manifests
	// and combined in an index.
	MaxBlobsPerManifest int

	// ArtifactType is the artifact type for manifests.
	ArtifactType string

	// ManifestAnnotations are annotations to add to all manifests.
	ManifestAnnotations map[string]string

	// ConfigAnnotations are annotations to set on the config descriptor of
	// each manifest, matching PackManifestOptions.ConfigAnnotations in the
	// ordinary push path.
	ConfigAnnotations map[string]string

	// PreserveEmptyDirs includes empty directories as empty manifests.
	PreserveEmptyDirs bool
}

// BuildResult contains the result of building manifests for a directory tree.
type BuildResult struct {
	// Root is the root descriptor (index or manifest).
	Root ocispec.Descriptor
	// AllDescriptors contains all created descriptors (blobs, manifests, indexes).
	AllDescriptors []ocispec.Descriptor
	// ManifestCount is the number of manifests created.
	ManifestCount int
	// IndexCount is the number of indexes created.
	IndexCount int
}

// Builder builds hierarchical OCI manifests and indexes from a directory tree.
type Builder struct {
	opts   BuilderOptions
	pusher content.Pusher
}

// NewBuilder creates a new manifest builder.
func NewBuilder(pusher content.Pusher, opts BuilderOptions) *Builder {
	if opts.MaxBlobsPerManifest <= 0 {
		opts.MaxBlobsPerManifest = 1000
	}
	return &Builder{
		opts:   opts,
		pusher: pusher,
	}
}

// BuildFromNode builds manifests/indexes for a directory node and its children.
// File descriptors must already be pushed and provided in fileDescs map (path -> descriptor).
// Returns the root descriptor for this node.
func (b *Builder) BuildFromNode(ctx context.Context, node *dir.Node, fileDescs map[string]ocispec.Descriptor) (*BuildResult, error) {
	if b.opts.ArtifactType != "" {
		if err := ValidateMediaType(b.opts.ArtifactType); err != nil {
			return nil, fmt.Errorf("invalid artifact type: %w", err)
		}
	}
	result := &BuildResult{}

	rootDesc, err := b.buildNode(ctx, node, fileDescs, result, true)
	if err != nil {
		return nil, err
	}

	result.Root = rootDesc
	return result, nil
}

// buildNode recursively builds manifests/indexes for a node.
func (b *Builder) buildNode(ctx context.Context, node *dir.Node, fileDescs map[string]ocispec.Descriptor, result *BuildResult, isRoot bool) (ocispec.Descriptor, error) {
	if !node.IsDir {
		// For files, just return the existing descriptor
		if desc, ok := fileDescs[node.Path]; ok {
			return desc, nil
		}
		return ocispec.Descriptor{}, nil
	}

	// Collect file descriptors for this directory
	var fileLayerDescs []ocispec.Descriptor
	for _, child := range node.Files() {
		if desc, ok := fileDescs[child.Path]; ok {
			fileLayerDescs = append(fileLayerDescs, desc)
		}
	}

	// Recursively build child directories
	var childDirDescs []ocispec.Descriptor
	for _, childDir := range node.Dirs() {
		childDesc, err := b.buildNode(ctx, childDir, fileDescs, result, false)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		if childDesc.Digest != "" {
			// Assign (replace) directory annotations. Do NOT merge with
			// maps.Copy, which would preserve any AnnotationTitle set by
			// createIndex and cause oras pull to treat the index as a file.
			childDesc.Annotations = descriptor.MakeDirectoryAnnotations(childDir.Path, childDir.Name)
			childDirDescs = append(childDirDescs, childDesc)
		}
	}

	// Determine what to create based on contents
	hasFiles := len(fileLayerDescs) > 0
	hasDirs := len(childDirDescs) > 0
	isEmpty := !hasFiles && !hasDirs

	if isEmpty {
		if b.opts.PreserveEmptyDirs {
			// Create empty manifest for empty directory
			return b.createManifest(ctx, nil, node, result, isRoot)
		}
		return ocispec.Descriptor{}, nil
	}

	// Check if we need chunking for files
	needsChunking := len(fileLayerDescs) > b.opts.MaxBlobsPerManifest

	if !hasDirs && !needsChunking {
		// Simple case: only files, fits in one manifest
		return b.createManifest(ctx, fileLayerDescs, node, result, isRoot)
	}

	// Complex case: need an index
	var indexManifests []ocispec.Descriptor

	// Handle file chunks
	if hasFiles {
		chunks := chunkDescriptors(fileLayerDescs, b.opts.MaxBlobsPerManifest)
		for i, chunkDescs := range chunks {
			// Do NOT set AnnotationTitle here. When this descriptor is stored
			// in the parent index's manifests list, oras pull's PostCopy calls
			// content.Successors on the index and then OnFilePulled for every
			// successor that has AnnotationTitle, which would try to create a
			// file named "<dirname>" and fail with "is a directory" when that
			// directory already exists on disk. The file layers inside the
			// manifest carry their own AnnotationTitle values which is what
			// drives actual file restoration during pull.
			annotations := make(map[string]string)
			maps.Copy(annotations, b.opts.ManifestAnnotations)
			if len(chunks) > 1 {
				annotations["org.oras.content.chunk.index"] = strconv.Itoa(i)
			}
			manifestDesc, err := b.createManifestWithAnnotations(ctx, chunkDescs, annotations, result)
			if err != nil {
				return ocispec.Descriptor{}, err
			}
			indexManifests = append(indexManifests, manifestDesc)
		}
	}

	// Add child directory indexes/manifests
	indexManifests = append(indexManifests, childDirDescs...)

	// Create the index
	return b.createIndex(ctx, indexManifests, node, result, isRoot)
}

// createManifest creates and pushes an OCI manifest.
func (b *Builder) createManifest(ctx context.Context, layers []ocispec.Descriptor, node *dir.Node, result *BuildResult, isRoot bool) (ocispec.Descriptor, error) {
	// Do NOT set ocispec.AnnotationTitle here. Structural descriptors
	// (directory manifests, including the root) must never carry a title:
	// oras pull materializes any titled descriptor as a file, so a titled
	// root manifest would be written out as a file named after the pushed
	// directory, colliding with same-named files or directories.
	annotations := make(map[string]string)
	if node != nil {
		maps.Copy(annotations, descriptor.MakeDirectoryAnnotations(node.Path, node.Name))
	}
	if isRoot {
		maps.Copy(annotations, descriptor.MakeRootAnnotations())
	}
	maps.Copy(annotations, b.opts.ManifestAnnotations)

	return b.createManifestWithAnnotations(ctx, layers, annotations, result)
}

// createManifestWithAnnotations creates and pushes an OCI manifest with specific annotations.
func (b *Builder) createManifestWithAnnotations(ctx context.Context, layers []ocispec.Descriptor, annotations map[string]string, result *BuildResult) (ocispec.Descriptor, error) {
	if layers == nil {
		layers = []ocispec.Descriptor{}
	}

	// Create empty config
	configBytes := []byte("{}")
	configDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageConfig, configBytes)
	if len(b.opts.ConfigAnnotations) > 0 {
		configDesc.Annotations = b.opts.ConfigAnnotations
	}

	// Push config
	if err := b.pusher.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		// Ignore if already exists
		if !errors.Is(err, errdef.ErrAlreadyExists) {
			return ocispec.Descriptor{}, err
		}
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    layers,
	}
	if b.opts.ArtifactType != "" {
		manifest.ArtifactType = b.opts.ArtifactType
	}
	if len(annotations) > 0 {
		manifest.Annotations = annotations
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
	manifestDesc.ArtifactType = b.opts.ArtifactType
	if descAnnotations := structuralAnnotations(annotations); len(descAnnotations) > 0 {
		manifestDesc.Annotations = descAnnotations
	}

	if err := b.pusher.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		if !errors.Is(err, errdef.ErrAlreadyExists) {
			return ocispec.Descriptor{}, err
		}
	}

	result.ManifestCount++
	result.AllDescriptors = append(result.AllDescriptors, manifestDesc)

	return manifestDesc, nil
}

// createIndex creates and pushes an OCI index.
func (b *Builder) createIndex(ctx context.Context, manifests []ocispec.Descriptor, node *dir.Node, result *BuildResult, isRoot bool) (ocispec.Descriptor, error) {
	// Do NOT add ocispec.AnnotationTitle here. If AnnotationTitle is present on
	// a descriptor stored in a parent index's manifests list, oras pull will
	// call metadataHandler.OnFilePulled with that name and try to create a file,
	// which fails with "is a directory" when a same-named directory exists.
	annotations := make(map[string]string)
	if node != nil {
		maps.Copy(annotations, descriptor.MakeDirectoryAnnotations(node.Path, node.Name))
	}
	if isRoot {
		maps.Copy(annotations, descriptor.MakeRootAnnotations())
	}
	maps.Copy(annotations, b.opts.ManifestAnnotations)

	index := ocispec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: manifests,
	}
	if b.opts.ArtifactType != "" {
		index.ArtifactType = b.opts.ArtifactType
	}
	if len(annotations) > 0 {
		index.Annotations = annotations
	}

	indexBytes, err := json.Marshal(index)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	indexDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexBytes)
	indexDesc.ArtifactType = b.opts.ArtifactType
	if descAnnotations := structuralAnnotations(annotations); len(descAnnotations) > 0 {
		indexDesc.Annotations = descAnnotations
	}

	if err := b.pusher.Push(ctx, indexDesc, bytes.NewReader(indexBytes)); err != nil {
		if !errors.Is(err, errdef.ErrAlreadyExists) {
			return ocispec.Descriptor{}, err
		}
	}

	result.IndexCount++
	result.AllDescriptors = append(result.AllDescriptors, indexDesc)

	return indexDesc, nil
}

// structuralAnnotations returns annotations with ocispec.AnnotationTitle
// removed. User manifest annotations may contain a title and are kept in the
// manifest content, but structural descriptors (directory manifests, chunk
// manifests, indexes, and the root) must never carry one: oras pull
// materializes any titled descriptor as a file, and several structural
// descriptors sharing the same title fail the pull with "duplicate name".
func structuralAnnotations(annotations map[string]string) map[string]string {
	if _, ok := annotations[ocispec.AnnotationTitle]; !ok {
		return annotations
	}
	filtered := make(map[string]string, len(annotations)-1)
	for k, v := range annotations {
		if k != ocispec.AnnotationTitle {
			filtered[k] = v
		}
	}
	return filtered
}

// chunkDescriptors splits a list of descriptors into chunks of at most maxSize.
func chunkDescriptors(descs []ocispec.Descriptor, maxSize int) [][]ocispec.Descriptor {
	if maxSize <= 0 || len(descs) <= maxSize {
		return [][]ocispec.Descriptor{descs}
	}

	var chunks [][]ocispec.Descriptor
	for i := 0; i < len(descs); i += maxSize {
		end := min(i+maxSize, len(descs))
		chunks = append(chunks, descs[i:end])
	}
	return chunks
}
