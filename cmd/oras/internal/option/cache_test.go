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

package option

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/content/oci"
)

var mockTarget oras.ReadOnlyTarget = memory.New()

func TestCache_CachedTarget(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ORAS_CACHE", tempDir)
	opts := Cache{}

	ctx := context.Background()
	src := memory.New()
	blob := []byte("hello cache")
	desc := content.NewDescriptorFromBytes("test", blob)
	if err := src.Push(ctx, desc, bytes.NewReader(blob)); err != nil {
		t.Fatal("error pushing to source, error =", err)
	}

	got, err := opts.CachedTarget(src)
	if err != nil {
		t.Fatal("Cache.CachedTarget() error=", err)
	}
	// oci.Store holds internal state that differs between instances opened on
	// the same root, so verify the caching behavior instead of comparing
	// against a separately constructed target.
	if reflect.DeepEqual(got, oras.ReadOnlyTarget(src)) {
		t.Fatal("Cache.CachedTarget() returned the source target, want a cached target")
	}
	fetched, err := content.FetchAll(ctx, got, desc)
	if err != nil {
		t.Fatal("error fetching through cached target, error =", err)
	}
	if !bytes.Equal(fetched, blob) {
		t.Fatalf("fetched content = %q, want %q", fetched, blob)
	}

	// The cache must not maintain index.json, which is not safe to share
	// between concurrent processes.
	if _, err := os.Stat(filepath.Join(tempDir, "index.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index.json in the cache root, os.Stat() error = %v, want %v", err, os.ErrNotExist)
	}

	ociStore, err := oci.New(tempDir)
	if err != nil {
		t.Fatal("error calling oci.New(), error =", err)
	}
	exists, err := ociStore.Exists(ctx, desc)
	if err != nil {
		t.Fatal("error checking cache, error =", err)
	}
	if !exists {
		t.Fatal("content was not stored in the cache")
	}
}

func TestCache_CachedTarget_sharedRoot(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ORAS_CACHE", tempDir)

	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := memory.New()
			blob := []byte(fmt.Sprintf("shared cache blob %d", i))
			desc := content.NewDescriptorFromBytes("test", blob)
			if err := src.Push(ctx, desc, bytes.NewReader(blob)); err != nil {
				errs <- err
				return
			}
			opts := Cache{}
			got, err := opts.CachedTarget(src)
			if err != nil {
				errs <- err
				return
			}
			fetched, err := content.FetchAll(ctx, got, desc)
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(fetched, blob) {
				errs <- fmt.Errorf("fetched content = %q, want %q", fetched, blob)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "index.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index.json in the shared cache root, os.Stat() error = %v, want %v", err, os.ErrNotExist)
	}
}

func TestCache_CachedTarget_emptyRoot(t *testing.T) {
	t.Setenv("ORAS_CACHE", "")
	opts := Cache{}

	got, err := opts.CachedTarget(mockTarget)
	if err != nil {
		t.Fatal("Cache.CachedTarget() error=", err)
	}
	if !reflect.DeepEqual(got, mockTarget) {
		t.Fatalf("Cache.CachedTarget() got %v, want %v", got, mockTarget)
	}
}
