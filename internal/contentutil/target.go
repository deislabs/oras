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
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/errdef"
)

type multiReadOnlyTarget struct {
	targets []oras.ReadOnlyTarget
}

// MultiReadOnlyTarget returns a ReadOnlyTarget that combines multiple targets.
func MultiReadOnlyTarget(targets ...oras.ReadOnlyTarget) oras.ReadOnlyTarget {
	return &multiReadOnlyTarget{
		targets: targets,
	}
}

// Fetch fetches the content from the targets in order and return first found
// content. If no content is found, it returns ErrNotFound.
func (m *multiReadOnlyTarget) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	lastErr := errdef.ErrNotFound
	for _, c := range m.targets {
		rc, err := c.Fetch(ctx, target)
		if err == nil {
			return rc, nil
		}
		if !errors.Is(err, errdef.ErrNotFound) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// Exists returns true if the content exists in any of the targets.
// multiReadOnlyTarget does not implement Exists() because it's read-only.
func (m *multiReadOnlyTarget) Exists(_ context.Context, _ ocispec.Descriptor) (bool, error) {
	return false, errors.New("MultiReadOnlyTarget.Exists() is not implemented")
}

// Resolve resolves the reference to a descriptor from the targets in order and
// return first found descriptor. If no descriptor is found, it returns
// ErrNotFound.
func (m *multiReadOnlyTarget) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	lastErr := errdef.ErrNotFound
	for _, c := range m.targets {
		desc, err := c.Resolve(ctx, ref)
		if err == nil {
			return desc, nil
		}
		if !errors.Is(err, errdef.ErrNotFound) {
			return ocispec.Descriptor{}, err
		}
		lastErr = err
	}
	return ocispec.Descriptor{}, lastErr
}
