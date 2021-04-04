package oras

import (
	"context"

	"github.com/containerd/containerd/remotes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Discover discovers artifacts referencing the specified artifact
func Discover(ctx context.Context, resolver remotes.Resolver, ref, artifactType string) (ocispec.Descriptor, []remotes.DiscoveredArtifact, error) {
	_, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	discoverer, err := resolver.Discoverer(ctx, ref)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	artifacts, err := discoverer.Discover(ctx, desc, artifactType)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	return desc, artifacts, err
}