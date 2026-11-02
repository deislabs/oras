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

package cache

import (
	"context"
	"io"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/registry"
)

type closer func() error

func (fn closer) Close() error {
	return fn()
}

// Cache target struct.
type target struct {
	oras.ReadOnlyTarget
	cache content.Storage
}

// New generates a new target storage with caching.
func New(source oras.ReadOnlyTarget, cache content.Storage) oras.ReadOnlyTarget {
	t := &target{
		ReadOnlyTarget: source,
		cache:          cache,
	}
	if refFetcher, ok := source.(registry.ReferenceFetcher); ok {
		return &referenceTarget{
			target:           t,
			ReferenceFetcher: refFetcher,
		}
	}
	return t
}

// Fetch fetches the content identified by the descriptor.
func (t *target) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	rc, err := t.cache.Fetch(ctx, target)
	if err == nil {
		// Fetch from cache
		return rc, nil
	}

	if rc, err = t.ReadOnlyTarget.Fetch(ctx, target); err != nil {
		return nil, err
	}

	// Fetch from origin with caching
	return t.cacheReadCloser(ctx, rc, target), nil
}

func (t *target) cacheReadCloser(ctx context.Context, rc io.ReadCloser, target ocispec.Descriptor) io.ReadCloser {
	pr, pw := io.Pipe()
	var wg sync.WaitGroup

	wg.Add(1)
	var pushErr error
	go func() {
		defer wg.Done()
		pushErr = t.cache.Push(ctx, target, pr)
		if pushErr != nil {
			pr.CloseWithError(pushErr)
		}
	}()

	return struct {
		io.Reader
		io.Closer
	}{
		Reader: io.TeeReader(rc, pw),
		Closer: closer(func() error {
			rcErr := rc.Close()
			if err := pw.Close(); err != nil {
				return err
			}
			wg.Wait()
			if pushErr != nil {
				return pushErr
			}
			return rcErr
		}),
	}
}

// Exists returns true if the described content exists.
func (t *target) Exists(ctx context.Context, desc ocispec.Descriptor) (bool, error) {
	exists, err := t.cache.Exists(ctx, desc)
	if err == nil && exists {
		return true, nil
	}
	return t.ReadOnlyTarget.Exists(ctx, desc)
}

// Cache referenceTarget struct.
type referenceTarget struct {
	*target
	registry.ReferenceFetcher
}

// FetchReference fetches the content identified by the reference from the
// remote and cache the fetched content.
// Cached content will only be read via Fetch, FetchReference will always fetch
// From origin.
func (t *referenceTarget) FetchReference(ctx context.Context, reference string) (ocispec.Descriptor, io.ReadCloser, error) {
	target, rc, err := t.ReferenceFetcher.FetchReference(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	// skip caching if the content already exists in cache
	exists, err := t.cache.Exists(ctx, target)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}
	if exists {
		err = rc.Close()
		if err != nil {
			return ocispec.Descriptor{}, nil, err
		}

		// get rc from the cache
		rc, err = t.cache.Fetch(ctx, target)
		if err != nil {
			return ocispec.Descriptor{}, nil, err
		}

		// no need to do tee'd push
		return target, rc, nil
	}

	// Fetch from origin with caching
	return target, t.cacheReadCloser(ctx, rc, target), nil
}
