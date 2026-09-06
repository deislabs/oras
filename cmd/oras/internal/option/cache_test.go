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
	"reflect"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/content/oci"
	"oras.land/oras/internal/cache"
)

var mockTarget oras.ReadOnlyTarget = memory.New()

func TestCache_CachedTarget(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ORAS_CACHE", tempDir)
	opts := Cache{}

	ociStore, err := oci.New(tempDir)
	if err != nil {
		t.Fatal("error calling oci.New(), error =", err)
	}
	want := cache.New(mockTarget, ociStore)

	got, err := opts.CachedTarget(mockTarget)
	if err != nil {
		t.Fatal("Cache.CachedTarget() error=", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Cache.CachedTarget() got %v, want %v", got, want)
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
