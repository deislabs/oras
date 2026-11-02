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

// Package mock contains mocking components for unit testing.
package mock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/registry/remote"
)

type content struct {
	ocispec.Descriptor
	blob []byte
}

type Repository struct {
	cas                map[string]content
	remote.Repository  // make tests compile
	isFetcher          bool
	isReferenceFetcher bool
	isResolver         bool
}

// WithFetch enables mocking for Fetch.
func (repo *Repository) WithFetch() *Repository {
	repo.isFetcher = true
	return repo
}

// WithFetchReference enables mocking for FetchReference.
func (repo *Repository) WithFetchReference() *Repository {
	repo.isReferenceFetcher = true
	return repo
}

// WithResolve enables mocking for Resolve.
func (repo *Repository) WithResolve() *Repository {
	repo.isResolver = true
	return repo
}

// New returns a new Repository struct.
func New() *Repository {
	return &Repository{}
}

// Blob mocks a content blob stored in content-addressable storage.
type Blob struct {
	Content   string
	MediaType string
	Tag       string
}

// Remount remounts the underlying CAS of the Repository.
func (repo *Repository) Remount(blobs []Blob) {
	repo.cas = make(map[string]content)
	for _, blob := range blobs {
		bytes := []byte(blob.Content)
		desc := ocispec.Descriptor{
			MediaType: blob.MediaType,
			Digest:    digest.FromBytes(bytes),
			Size:      int64(len(bytes)),
		}
		repo.cas[string(desc.Digest)] = content{desc, bytes}
		if blob.Tag != "" {
			repo.cas[blob.Tag] = content{desc, bytes}
		}
	}
}

var errNotImplemented = errors.New("not implemented")

// FetchReference mocks the fetching via a reference ref.
func (repo *Repository) FetchReference(_ context.Context, ref string) (ocispec.Descriptor, io.ReadCloser, error) {
	if !repo.isReferenceFetcher {
		return ocispec.Descriptor{}, nil, errNotImplemented
	}

	if c, ok := repo.cas[ref]; ok {
		return c.Descriptor, io.NopCloser(bytes.NewReader(c.blob)), nil
	}
	return ocispec.Descriptor{}, nil, fmt.Errorf("got unexpected reference %q", ref)
}

// Fetch mocks fetching the target descriptor.
func (repo *Repository) Fetch(_ context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	if !repo.isFetcher {
		return nil, errNotImplemented
	}

	if r, ok := repo.cas[target.Digest.String()]; ok {
		return io.NopCloser(bytes.NewReader(r.blob)), nil
	}
	return nil, fmt.Errorf("unexpected descriptor %v", target)
}

// Resolve mocks resolving via a reference.
func (repo *Repository) Resolve(_ context.Context, reference string) (ocispec.Descriptor, error) {
	if !repo.isResolver {
		return ocispec.Descriptor{}, errNotImplemented
	}

	if r, ok := repo.cas[reference]; ok {
		return r.Descriptor, nil
	}
	return ocispec.Descriptor{}, fmt.Errorf("unexpected reference %v", reference)
}
