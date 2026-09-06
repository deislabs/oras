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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/errdef"
)

// fakeReadOnlyTarget is an oras.ReadOnlyTarget that either serves fixed
// content and a fixed descriptor, or fails every call with err. It records
// whether it was called.
type fakeReadOnlyTarget struct {
	content []byte
	desc    ocispec.Descriptor
	err     error
	called  bool
}

func (f *fakeReadOnlyTarget) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.content)), nil
}

func (f *fakeReadOnlyTarget) Exists(_ context.Context, _ ocispec.Descriptor) (bool, error) {
	f.called = true
	return f.err == nil, f.err
}

func (f *fakeReadOnlyTarget) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	f.called = true
	if f.err != nil {
		return ocispec.Descriptor{}, f.err
	}
	return f.desc, nil
}

var errBackend = errors.New("backend unavailable")

func notFound() *fakeReadOnlyTarget {
	return &fakeReadOnlyTarget{err: fmt.Errorf("lookup: %w", errdef.ErrNotFound)}
}

func TestMultiReadOnlyTarget_Fetch(t *testing.T) {
	ctx := context.Background()
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("manifest"),
		Size:      8,
	}

	t.Run("falls through targets that do not have the content", func(t *testing.T) {
		second := &fakeReadOnlyTarget{content: []byte("manifest")}
		third := &fakeReadOnlyTarget{content: []byte("other")}
		rc, err := MultiReadOnlyTarget(notFound(), second, third).Fetch(ctx, desc)
		if err != nil {
			t.Fatalf("Fetch() error = %v, want nil", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("reading fetched content: %v", err)
		}
		if string(got) != "manifest" {
			t.Errorf("Fetch() content = %q, want %q", got, "manifest")
		}
		if third.called {
			t.Error("Fetch() consulted a target after the content was found")
		}
	})

	t.Run("stops at an error other than not found", func(t *testing.T) {
		next := &fakeReadOnlyTarget{content: []byte("manifest")}
		_, err := MultiReadOnlyTarget(&fakeReadOnlyTarget{err: errBackend}, next).Fetch(ctx, desc)
		if !errors.Is(err, errBackend) {
			t.Errorf("Fetch() error = %v, want %v", err, errBackend)
		}
		if next.called {
			t.Error("Fetch() consulted the next target after a non-not-found error")
		}
	})

	t.Run("returns not found when no target has the content", func(t *testing.T) {
		_, err := MultiReadOnlyTarget(notFound(), notFound()).Fetch(ctx, desc)
		if !errors.Is(err, errdef.ErrNotFound) {
			t.Errorf("Fetch() error = %v, want %v", err, errdef.ErrNotFound)
		}
	})

	t.Run("returns not found without targets", func(t *testing.T) {
		_, err := MultiReadOnlyTarget().Fetch(ctx, desc)
		if !errors.Is(err, errdef.ErrNotFound) {
			t.Errorf("Fetch() error = %v, want %v", err, errdef.ErrNotFound)
		}
	})
}

func TestMultiReadOnlyTarget_Resolve(t *testing.T) {
	ctx := context.Background()
	want := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("manifest"),
		Size:      8,
	}

	t.Run("falls through targets that do not have the reference", func(t *testing.T) {
		third := &fakeReadOnlyTarget{desc: ocispec.Descriptor{Digest: digest.FromString("other")}}
		got, err := MultiReadOnlyTarget(notFound(), &fakeReadOnlyTarget{desc: want}, third).Resolve(ctx, "v1")
		if err != nil {
			t.Fatalf("Resolve() error = %v, want nil", err)
		}
		if got.Digest != want.Digest {
			t.Errorf("Resolve() digest = %v, want %v", got.Digest, want.Digest)
		}
		if third.called {
			t.Error("Resolve() consulted a target after the reference was resolved")
		}
	})

	t.Run("stops at an error other than not found", func(t *testing.T) {
		next := &fakeReadOnlyTarget{desc: want}
		got, err := MultiReadOnlyTarget(&fakeReadOnlyTarget{err: errBackend}, next).Resolve(ctx, "v1")
		if !errors.Is(err, errBackend) {
			t.Errorf("Resolve() error = %v, want %v", err, errBackend)
		}
		if got.Digest != "" {
			t.Errorf("Resolve() descriptor = %v, want the zero descriptor", got)
		}
		if next.called {
			t.Error("Resolve() consulted the next target after a non-not-found error")
		}
	})

	t.Run("returns not found when no target has the reference", func(t *testing.T) {
		_, err := MultiReadOnlyTarget(notFound(), notFound()).Resolve(ctx, "v1")
		if !errors.Is(err, errdef.ErrNotFound) {
			t.Errorf("Resolve() error = %v, want %v", err, errdef.ErrNotFound)
		}
	})

	t.Run("returns not found without targets", func(t *testing.T) {
		_, err := MultiReadOnlyTarget().Resolve(ctx, "v1")
		if !errors.Is(err, errdef.ErrNotFound) {
			t.Errorf("Resolve() error = %v, want %v", err, errdef.ErrNotFound)
		}
	})
}

func TestMultiReadOnlyTarget_Exists(t *testing.T) {
	target := &fakeReadOnlyTarget{}
	exists, err := MultiReadOnlyTarget(target).Exists(context.Background(), ocispec.Descriptor{})
	if err == nil {
		t.Error("Exists() error = nil, want an error because it is not implemented")
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
	if target.called {
		t.Error("Exists() consulted a target, but it is not implemented")
	}
}
