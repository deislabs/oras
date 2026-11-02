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
	"bytes"
	"context"
	_ "crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/registry"
	"github.com/oras-project/oras-go/v3/registry/remote"
)

func TestProxy_fetchCache(t *testing.T) {
	blob := []byte("hello world")
	desc := ocispec.Descriptor{
		MediaType: "test",
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
	}

	target := memory.New()
	p := struct {
		oras.Target
		oras.ReadOnlyTarget
		cache content.Storage
	}{
		target,
		target,
		memory.New(),
	}

	ctx := context.Background()

	err := p.Push(ctx, desc, bytes.NewReader(blob))
	if err != nil {
		t.Fatal("Proxy.Push() error =", err)
	}

	// first fetch
	exists, err := p.Target.Exists(ctx, desc)
	if err != nil {
		t.Fatal("Proxy.Exists() error =", err)
	}
	if !exists {
		t.Errorf("Proxy.Exists() = %v, want %v", exists, true)
	}
	got, err := content.FetchAll(ctx, p.Target, desc)
	if err != nil {
		t.Fatal("Proxy.Fetch() error =", err)
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("Proxy.Fetch() = %v, want %v", got, blob)
	}

	// repeated fetch should not touch base CAS
	// nil base will generate panic if the base CAS is touched
	p.Target = nil

	exists, err = p.ReadOnlyTarget.Exists(ctx, desc)
	if err != nil {
		t.Fatal("Proxy.Exists() error =", err)
	}
	if !exists {
		t.Errorf("Proxy.Exists() = %v, want %v", exists, true)
	}
	got, err = content.FetchAll(ctx, p.ReadOnlyTarget, desc)
	if err != nil {
		t.Fatal("Proxy.Fetch() error =", err)
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("Proxy.Fetch() = %v, want %v", got, blob)
	}
}

func TestProxy_pushPassThrough(t *testing.T) {
	blob := []byte("hello world")
	desc := ocispec.Descriptor{
		MediaType: "test",
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
	}

	p := struct {
		oras.Target
		cache content.Storage
	}{
		memory.New(),
		memory.New(),
	}
	ctx := context.Background()

	// before push
	exists, err := p.Exists(ctx, desc)
	if err != nil {
		t.Fatal("Proxy.Exists() error =", err)
	}
	if exists {
		t.Errorf("Proxy.Exists() = %v, want %v", exists, false)
	}

	// push
	err = p.Push(ctx, desc, bytes.NewReader(blob))
	if err != nil {
		t.Fatal("Proxy.Push() error =", err)
	}

	// after push
	exists, err = p.Exists(ctx, desc)
	if err != nil {
		t.Fatal("Proxy.Exists() error =", err)
	}
	if !exists {
		t.Errorf("Proxy.Exists() = %v, want %v", exists, true)
	}
	got, err := content.FetchAll(ctx, p, desc)
	if err != nil {
		t.Fatal("Proxy.Fetch() error =", err)
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("Proxy.Fetch() = %v, want %v", got, blob)
	}
}

func TestProxy_fetchReference(t *testing.T) {
	// mocked variables
	blob := []byte("{}")
	repoName := "test/repo"
	tagName := "test-tag"
	mediaType := ocispec.MediaTypeImageManifest
	digest := digest.FromBytes(blob)
	desc := ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int64(len(blob)),
	}

	// mocked remote registry
	var requestCount, wantRequestCount int64
	var successCount, wantSuccessCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		if r.Method == http.MethodGet &&
			(r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoName, tagName) ||
				r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoName, digest)) {
			w.Header().Set("Content-Type", mediaType)
			w.Header().Set("Docker-Content-Digest", digest.String())
			w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
			w.WriteHeader(http.StatusOK)
			// write data to the response if this is the first request
			if requestCount == 1 {
				if _, err := w.Write(blob); err != nil {
					t.Errorf("Error writing blobs: %v", err)
				}
			}
			atomic.AddInt64(&successCount, 1)
			return
		}
		t.Errorf("unexpected access: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	uri, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("invalid test http server: %v", err)
	}
	repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", uri.Host, repoName))
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	repo.Registry.PlainHTTP = true
	p := New(repo, memory.New())
	ctx := context.Background()

	// first fetch reference
	gotDesc, rc, err := p.(registry.ReferenceFetcher).FetchReference(ctx, tagName)
	if err != nil {
		t.Fatal("ReferenceTarget.FetchReference() error =", err)
	}
	if !reflect.DeepEqual(gotDesc, desc) {
		t.Fatalf("ReferenceTarget.FetchReference() got %v, want %v", gotDesc, desc)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal("io.ReadAll() error =", err)
	}
	err = rc.Close()
	if err != nil {
		t.Error("ReferenceTarget.FetchReference().Close() error =", err)
	}

	if !bytes.Equal(got, blob) {
		t.Errorf("ReferenceTarget.Fetch() = %v, want %v", got, blob)
	}
	if wantRequestCount++; requestCount != wantRequestCount {
		t.Errorf("unexpected number of requests: %d, want %d", requestCount, wantRequestCount)
	}
	if wantSuccessCount++; successCount != wantSuccessCount {
		t.Errorf("unexpected number of successful requests: %d, want %d", successCount, wantSuccessCount)
	}

	// second fetch reference, should get the rc from the cache
	gotDesc, rc, err = p.(registry.ReferenceFetcher).FetchReference(ctx, tagName)
	if err != nil {
		t.Fatal("ReferenceTarget.FetchReference() error =", err)
	}
	if !reflect.DeepEqual(gotDesc, desc) {
		t.Fatalf("ReferenceTarget.FetchReference() got %v, want %v", gotDesc, desc)
	}
	got, err = io.ReadAll(rc)
	if err != nil {
		t.Fatal("io.ReadAll() error =", err)
	}
	err = rc.Close()
	if err != nil {
		t.Error("ReferenceTarget.FetchReference().Close() error =", err)
	}

	if !bytes.Equal(got, blob) {
		t.Errorf("ReferenceTarget.Fetch() = %v, want %v", got, blob)
	}
	if wantRequestCount++; requestCount != wantRequestCount {
		t.Errorf("unexpected number of requests: %d, want %d", requestCount, wantRequestCount)
	}
	if wantSuccessCount++; successCount != wantSuccessCount {
		t.Errorf("unexpected number of successful requests: %d, want %d", successCount, wantSuccessCount)
	}

	// repeated fetch should not touch base CAS
	p.(*referenceTarget).ReadOnlyTarget = nil
	got, err = content.FetchAll(ctx, p, desc)
	if err != nil {
		t.Fatal("ReferenceTarget.Fetch() error =", err)
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("ReferenceTarget.Fetch() = %v, want %v", got, blob)
	}
	if requestCount != wantRequestCount {
		t.Errorf("unexpected number of requests: %d, want %d", requestCount, wantRequestCount)
	}
	if successCount != wantSuccessCount {
		t.Errorf("unexpected number of successful requests: %d, want %d", successCount, wantSuccessCount)
	}
}
