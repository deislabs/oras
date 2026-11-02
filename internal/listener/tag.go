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

package listener

import (
	"context"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/registry"
)

// NewTagListener creates a wrapper type for printing a tag status and hint.
// It can only be used for oras.TagBytes and oras.TagBytesN.
func NewTagListener(target oras.Target, onTagging, onTagged func(desc ocispec.Descriptor, tag string) error) oras.Target {
	if repo, ok := target.(registry.Repository); ok {
		return &tagListenerForRepository{
			Repository: repo,
			onTagging:  onTagging,
			onTagged:   onTagged,
		}
	}
	return &tagListenerForTarget{
		Target:    target,
		onTagging: onTagging,
		onTagged:  onTagged,
	}
}

// NewTaggedListener creates a wrapper type for printing all tagged statuses.
// It can only be used for oras.TagBytes and oras.TagBytesN.
func NewTaggedListener(target oras.Target, onTagged func(desc ocispec.Descriptor, tag string) error) oras.Target {
	return NewTagListener(target, nil, onTagged)
}

type tagListenerForRepository struct {
	registry.Repository
	onTagging func(desc ocispec.Descriptor, tag string) error
	onTagged  func(desc ocispec.Descriptor, tag string) error
}

// PushReference overrides Repository.PushReference method to print off which tag(s) were added successfully.
func (l *tagListenerForRepository) PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	if l.onTagging != nil {
		if err := l.onTagging(expected, reference); err != nil {
			return err
		}
	}
	if err := l.Repository.PushReference(ctx, expected, content, reference); err != nil {
		return err
	}
	return l.onTagged(expected, reference)
}

type tagListenerForTarget struct {
	oras.Target
	onTagging func(desc ocispec.Descriptor, tag string) error
	onTagged  func(desc ocispec.Descriptor, tag string) error
}

// Tag tags a descriptor with a reference string.
func (l *tagListenerForTarget) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	if l.onTagging != nil {
		if err := l.onTagging(desc, reference); err != nil {
			return err
		}
	}
	if err := l.Target.Tag(ctx, desc, reference); err != nil {
		return err
	}
	return l.onTagged(desc, reference)
}
