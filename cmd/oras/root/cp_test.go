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

package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/display/metadata"
	"oras.land/oras/cmd/oras/internal/display/status"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/internal/testutils"
)

var (
	memStore        *memory.Store
	memDesc         ocispec.Descriptor
	manifestContent = []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.unknown.artifact.v1","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2,"data":"e30="},"layers":[{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2,"data":"e30="}]}`)
	manifestDigest  = "sha256:1bb053792feb8d8d590001c212f2defad9277e091d2aa868cde2879ff41abb1b"
	configContent   = []byte("{}")
	configDigest    = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	configMediaType = "application/vnd.oci.empty.v1+json"
	host            string
	repoFrom        = "from"
	repoTo          = "to"
)

func TestMain(m *testing.M) {
	// memory store for testing
	memStore = memory.New()
	content := []byte("test")
	r := bytes.NewReader(content)
	memDesc = ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
	}
	if err := memStore.Push(context.Background(), memDesc, r); err != nil {
		fmt.Println("Setup failed:", err)
		os.Exit(1)
	}
	if err := memStore.Tag(context.Background(), memDesc, memDesc.Digest.String()); err != nil {
		fmt.Println("Setup failed:", err)
		os.Exit(1)
	}

	// test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoFrom, manifestDigest) &&
			r.Method == http.MethodHead:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoFrom, manifestDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			_, _ = w.Write(manifestContent)
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/%s", repoFrom, configDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprint(len(configContent)))
			_, _ = w.Write(configContent)
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoTo, manifestDigest) &&
			r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/%s", repoTo, configDigest) &&
			r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/uploads/", repoTo) &&
			r.URL.Query().Get("mount") == configDigest &&
			r.URL.Query().Get("from") == repoFrom &&
			r.Method == http.MethodPost:
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoTo, configDigest))
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoTo, manifestDigest) &&
			r.Method == http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoTo, manifestDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			_, _ = w.Write(manifestContent)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}))
	defer ts.Close()
	uri, _ := url.Parse(ts.URL)
	host = "localhost:" + uri.Port()
	m.Run()
}

func Test_doCopy(t *testing.T) {
	// prepare
	var opts copyOptions
	opts.From.Reference = memDesc.Digest.String()
	dst := memory.New()
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}
	// test
	_, err := doCopy(context.Background(), handler, memStore, dst, &opts)
	if err != nil {
		t.Fatal(err)
	}
	// validate
	if got, want := recorder.Snapshot(), []string{"PreCopy", "PostCopy"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_doCopy_skipped(t *testing.T) {
	// prepare
	var opts copyOptions
	opts.From.Reference = memDesc.Digest.String()
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}

	// test
	_, err := doCopy(context.Background(), handler, memStore, memStore, &opts)
	if err != nil {
		t.Fatal(err)
	}
	// validate
	if got, want := recorder.Snapshot(), []string{"OnCopySkipped"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_copyAndFinalize_handlerCalls(t *testing.T) {
	var opts copyOptions
	opts.From.Reference = memDesc.Digest.String()
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingCopyMetadataHandler{Recorder: recorder}

	if err := copyAndFinalize(context.Background(), statusHandler, metadataHandler, memStore, memory.New(), &opts, true); err != nil {
		t.Fatalf("copyAndFinalize() error = %v", err)
	}
	if got, want := recorder.Snapshot(), []string{"PreCopy", "PostCopy", "OnCopied", "Render"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_copyAndFinalize_onCopiedError(t *testing.T) {
	wantedErr := errors.New("metadata copy failed")
	var opts copyOptions
	opts.From.Reference = memDesc.Digest.String()
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingCopyMetadataHandler{Recorder: recorder, CopiedErr: wantedErr}

	err := copyAndFinalize(context.Background(), statusHandler, metadataHandler, memStore, memory.New(), &opts, true)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("copyAndFinalize() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"PreCopy", "PostCopy", "OnCopied"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_doCopy_mounted(t *testing.T) {
	// prepare
	_, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	var opts copyOptions
	opts.From.Reference = manifestDigest
	// mocked repositories
	from, err := remote.NewRepository(fmt.Sprintf("%s/%s", host, repoFrom))
	if err != nil {
		t.Fatal(err)
	}
	from.PlainHTTP = true
	to, err := remote.NewRepository(fmt.Sprintf("%s/%s", host, repoTo))
	if err != nil {
		t.Fatal(err)
	}
	to.PlainHTTP = true
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}

	// test
	_, err = doCopy(context.Background(), handler, from, to, &opts)
	if err != nil {
		t.Fatal(err)
	}
	// validate
	if got, want := recorder.Snapshot(), []string{"OnMounted", "PreCopy", "PostCopy"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_doCopy_mountFallback(t *testing.T) {
	// Test that copy falls back to regular upload when mount fails with 401/403
	repoFromFallback := "from-fallback"
	repoToFallback := "to-fallback"

	var uploadSessionStarted atomic.Bool
	var blobUploaded atomic.Bool

	// test server that returns 401 for mount but allows regular upload
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoFromFallback, manifestDigest) &&
			r.Method == http.MethodHead:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoFromFallback, manifestDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			_, _ = w.Write(manifestContent)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/%s", repoFromFallback, configDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprint(len(configContent)))
			_, _ = w.Write(configContent)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoToFallback, manifestDigest) &&
			r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/%s", repoToFallback, configDigest) &&
			r.Method == http.MethodHead:
			// check if blob exists before upload - not found initially
			if blobUploaded.Load() {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Length", fmt.Sprint(len(configContent)))
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/uploads/", repoToFallback) &&
			r.URL.Query().Get("mount") != "" &&
			r.Method == http.MethodPost:
			// Return 401 Unauthorized for mount requests to simulate permission denial
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == fmt.Sprintf("/v2/%s/blobs/uploads/", repoToFallback) &&
			r.URL.Query().Get("mount") == "" &&
			r.Method == http.MethodPost:
			// Regular blob upload initiation - allow this
			uploadSessionStarted.Store(true)
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/session123", repoToFallback))
			w.WriteHeader(http.StatusAccepted)
		case strings.HasPrefix(r.URL.Path, fmt.Sprintf("/v2/%s/blobs/uploads/", repoToFallback)) &&
			r.Method == http.MethodPut:
			// Blob upload completion
			blobUploaded.Store(true)
			w.Header().Set("Docker-Content-Digest", configDigest)
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoToFallback, manifestDigest) &&
			r.Method == http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == fmt.Sprintf("/v2/%s/manifests/%s", repoToFallback, manifestDigest) &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", fmt.Sprint(len(manifestContent)))
			_, _ = w.Write(manifestContent)
		default:
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}))
	defer ts.Close()

	uri, _ := url.Parse(ts.URL)
	testHost := "localhost:" + uri.Port()

	// prepare
	_, child, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Close() }()
	var opts copyOptions
	opts.From.Reference = manifestDigest
	// mocked repositories
	from, err := remote.NewRepository(fmt.Sprintf("%s/%s", testHost, repoFromFallback))
	if err != nil {
		t.Fatal(err)
	}
	from.PlainHTTP = true
	to, err := remote.NewRepository(fmt.Sprintf("%s/%s", testHost, repoToFallback))
	if err != nil {
		t.Fatal(err)
	}
	to.PlainHTTP = true
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingCopyStatusHandler{Recorder: recorder}

	// test
	_, err = doCopy(context.Background(), handler, from, to, &opts)
	if err != nil {
		t.Fatal(err)
	}

	// validate that regular upload was used (fallback succeeded)
	if !uploadSessionStarted.Load() {
		t.Error("expected regular upload session to be started after mount failure")
	}
	if !blobUploaded.Load() {
		t.Error("expected blob to be uploaded via regular upload after mount failure")
	}
	// validate output shows "Copied" instead of "Mounted"
	if got, want := recorder.Snapshot(), []string{"PreCopy", "PostCopy", "PreCopy", "PostCopy"}; !slices.Equal(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func Test_prepareCopyOption_nonIndex(t *testing.T) {
	ctx := context.Background()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
	}
	if _, _, err := prepareCopyOption(ctx, nil, nil, root, oras.ExtendedCopyGraphOptions{}); err != nil {
		t.Errorf("prepareCopyOption() error = %v, wantErr false", err)
	}
}

var errMockedFetch = fmt.Errorf("fetch error")

// fetchFailingReadOnlyGraphTarget is a mock implementation of oras.ReadOnlyGraphTarget
type fetchFailingReadOnlyGraphTarget struct {
	oras.ReadOnlyGraphTarget
}

// Fetch simulates a failure when fetching content from the source.
func (m *fetchFailingReadOnlyGraphTarget) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, errMockedFetch
}

func Test_prepareCopyOption_fetchFailure(t *testing.T) {
	ctx := context.Background()
	src := &fetchFailingReadOnlyGraphTarget{}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString("nonexistent"),
		Size:      int64(len("nonexistent")),
	}

	if _, _, err := prepareCopyOption(ctx, src, dst, root, oras.ExtendedCopyGraphOptions{}); !errors.Is(err, errMockedFetch) {
		t.Errorf("prepareCopyOption() error = %v, want %v", err, errMockedFetch)
	}
}

func Test_recursiveCopy_prepareCopyOptionFailure(t *testing.T) {
	ctx := context.Background()
	src := &fetchFailingReadOnlyGraphTarget{}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString("nonexistent"),
		Size:      int64(len("nonexistent")),
	}

	if _, _, err := prepareCopyOption(ctx, src, dst, root, oras.ExtendedCopyGraphOptions{}); !errors.Is(err, errMockedFetch) {
		t.Errorf("prepareCopyOption() error = %v, want %v", err, errMockedFetch)
	}
}

// invalidJSONReadOnlyGraphTarget is a mock implementation of oras.ReadOnlyGraphTarget
// that returns invalid JSON data to simulate a JSON unmarshalling failure.
type invalidJSONReadOnlyGraphTarget struct {
	oras.ReadOnlyGraphTarget
}

// Fetch simulates a successful fetch of invalid JSON data.
func (m *invalidJSONReadOnlyGraphTarget) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	// Return invalid JSON data
	return io.NopCloser(strings.NewReader("invalid-json")), nil
}

func Test_prepareCopyOption_jsonUnmarshalFailure(t *testing.T) {
	ctx := context.Background()
	src := &invalidJSONReadOnlyGraphTarget{}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString("invalid-json"),
		Size:      int64(len("invalid-json")),
	}
	_, _, err := prepareCopyOption(ctx, src, dst, root, oras.ExtendedCopyGraphOptions{})
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Errorf("prepareCopyOption() error = %v, want json.SyntaxError", err)
	}
}

// mockReferrersFailingSource is a mock implementation of oras.ReadOnlyGraphTarget
// that simulates a failure when fetching referrers.
type mockReferrersFailingSource struct {
	oras.ReadOnlyGraphTarget
	indexContent string
}

// Fetch simulates successful fetching of index content.
func (m *mockReferrersFailingSource) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	// Return valid JSON data to pass the fetch step
	return io.NopCloser(strings.NewReader(m.indexContent)), nil
}

func Test_prepareCopyOption_referrersFailure(t *testing.T) {
	ctx := context.Background()
	mockedIndex := `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2}]}`
	src := &mockReferrersFailingSource{indexContent: mockedIndex}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString(mockedIndex),
		Size:      int64(len(mockedIndex)),
	}
	errMockedReferrers := fmt.Errorf("failed to get referrers")
	opts := oras.ExtendedCopyGraphOptions{
		FindPredecessors: func(_ context.Context, _ content.ReadOnlyGraphStorage, _ ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return nil, errMockedReferrers
		},
	}

	if _, _, err := prepareCopyOption(ctx, src, dst, root, opts); !errors.Is(err, errMockedReferrers) {
		t.Errorf("prepareCopyOption() error = %v, wantErr %v", err, errMockedReferrers)
	}
}

func Test_prepareCopyOption_referrersFailureOnIndex(t *testing.T) {
	ctx := context.Background()
	mockedIndex := `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2}]}`
	src := &mockReferrersFailingSource{indexContent: mockedIndex}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString(mockedIndex),
		Size:      int64(len(mockedIndex)),
	}
	errMockedReferrers := fmt.Errorf("failed to get referrers")
	opts := oras.ExtendedCopyGraphOptions{
		FindPredecessors: func(_ context.Context, _ content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			if desc.MediaType == ocispec.MediaTypeImageIndex {
				return nil, errMockedReferrers
			}
			entry := ocispec.Descriptor{}
			return []ocispec.Descriptor{entry}, nil
		},
	}

	if _, _, err := prepareCopyOption(ctx, src, dst, root, opts); !errors.Is(err, errMockedReferrers) {
		t.Errorf("prepareCopyOption() error = %v, wantErr %v", err, errMockedReferrers)
	}
}

func Test_prepareCopyOption_noReferrers(t *testing.T) {
	ctx := context.Background()
	mockedIndex := `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2}]}`
	src := &mockReferrersFailingSource{indexContent: mockedIndex}
	dst := memory.New()
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString(mockedIndex),
		Size:      int64(len(mockedIndex)),
	}
	opts := oras.ExtendedCopyGraphOptions{
		FindPredecessors: func(_ context.Context, _ content.ReadOnlyGraphStorage, _ ocispec.Descriptor) ([]ocispec.Descriptor, error) {
			return nil, nil
		},
	}

	if _, _, err := prepareCopyOption(ctx, src, dst, root, opts); err != nil {
		t.Errorf("prepareCopyOption() error = %v, wantErr false", err)
	}
}

func Test_getMountPoint(t *testing.T) {
	registry1Repo1 := &remote.Repository{}
	registry1Repo1.Reference.Registry = "localhost:5000"
	registry1Repo1.Reference.Repository = "repo1"

	registry1Repo2 := &remote.Repository{}
	registry1Repo2.Reference.Registry = "localhost:5000"
	registry1Repo2.Reference.Repository = "repo2"

	registry2Repo1 := &remote.Repository{}
	registry2Repo1.Reference.Registry = "localhost:6000"
	registry2Repo1.Reference.Repository = "repo1"

	tests := []struct {
		name         string
		src          oras.ReadOnlyGraphTarget
		dst          oras.GraphTarget
		fromUsername string
		fromPassword string
		toUsername   string
		toPassword   string
		wantRepo     bool
		wantMount    string
	}{
		{
			name:         "should mount: both remote, same registry, same credentials",
			src:          registry1Repo1,
			dst:          registry1Repo2,
			fromUsername: "user1",
			fromPassword: "pass1",
			toUsername:   "user1",
			toPassword:   "pass1",
			wantRepo:     true,
			wantMount:    "repo1",
		},
		{
			name:         "should not mount: both remote, same registry, different credentials",
			src:          registry1Repo1,
			dst:          registry1Repo2,
			fromUsername: "user1",
			fromPassword: "pass1",
			toUsername:   "user2",
			toPassword:   "pass2",
			wantRepo:     false,
			wantMount:    "",
		},
		{
			name:         "should not mount: both remote, different registries",
			src:          registry1Repo1,
			dst:          registry2Repo1,
			fromUsername: "user1",
			fromPassword: "pass1",
			toUsername:   "user3",
			toPassword:   "pass3",
			wantRepo:     false,
			wantMount:    "",
		},
		{
			name:       "should not mount: source is not remote",
			src:        memStore,
			dst:        registry1Repo1,
			toUsername: "user1",
			toPassword: "pass1",
			wantRepo:   false,
			wantMount:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &copyOptions{}
			opts.From.Username = tt.fromUsername
			opts.From.Secret = tt.fromPassword
			opts.To.Username = tt.toUsername
			opts.To.Secret = tt.toPassword

			gotMount, gotRepo := getMountPoint(tt.src, tt.dst, opts)
			if gotRepo != tt.wantRepo {
				t.Errorf("checkMount() gotRepo = %v, want %v", gotRepo, tt.wantRepo)
			}
			if gotMount != tt.wantMount {
				t.Errorf("checkMount() gotRepo = %v, want %v", gotMount, tt.wantMount)
			}
		})
	}

	filtered := &filteredIndexSource{ReadOnlyGraphTarget: registry1Repo1}
	gotMount, gotRepo := getMountPoint(filtered, registry1Repo2, &copyOptions{})
	if !gotRepo || gotMount != "repo1" {
		t.Fatalf("getMountPoint() through filtered source = (%q, %v), want (%q, true)", gotMount, gotRepo, "repo1")
	}
}

// tagFailingTarget is a mock implementation of oras.Target whose Tag always
// fails, simulating a destination registry that rejects the root tag.
type tagFailingTarget struct {
	oras.Target
}

// Tag simulates a not found failure at the destination.
func (t *tagFailingTarget) Tag(_ context.Context, _ ocispec.Descriptor, _ string) error {
	return errdef.ErrNotFound
}

func Test_recursiveCopy_tagFailure(t *testing.T) {
	ctx := context.Background()
	dst := &tagFailingTarget{Target: memory.New()}

	err := recursiveCopy(ctx, memStore, dst, "v1", memDesc, oras.DefaultExtendedCopyGraphOptions)
	if err == nil {
		t.Fatal("recursiveCopy() error = nil, wantErr true")
	}
	var copyErr *oras.CopyError
	if !errors.As(err, &copyErr) {
		t.Fatalf("recursiveCopy() error = %v, want *oras.CopyError", err)
	}
	if copyErr.Op != "Tag" {
		t.Errorf("recursiveCopy() error Op = %q, want %q", copyErr.Op, "Tag")
	}
	if copyErr.Origin != oras.CopyErrorOriginDestination {
		t.Errorf("recursiveCopy() error Origin = %v, want %v", copyErr.Origin, oras.CopyErrorOriginDestination)
	}
	if !errors.Is(err, errdef.ErrNotFound) {
		t.Errorf("recursiveCopy() error = %v, want to wrap %v", err, errdef.ErrNotFound)
	}
}

// newStrandedRootSource builds a source holding an index with a single child
// manifest, together with the FindPredecessors that a registry without
// Referrers API support produces for it: resolving the referrers tag
// `sha256-<hex>` as the digest `sha256:<hex>` answers with the index itself,
// so the index's own child comes back as its referrer.
func newStrandedRootSource(t *testing.T) (*memory.Store, ocispec.Descriptor, oras.ExtendedCopyGraphOptions) {
	t.Helper()
	ctx := context.Background()
	src := memory.New()

	configDesc := ocispec.Descriptor{
		MediaType: configMediaType,
		Digest:    digest.FromBytes(configContent),
		Size:      int64(len(configContent)),
	}
	if err := src.Push(ctx, configDesc, bytes.NewReader(configContent)); err != nil {
		t.Fatal(err)
	}

	child := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[]}`,
		ocispec.MediaTypeImageManifest, configDesc.MediaType, configDesc.Digest, configDesc.Size))
	childDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(child),
		Size:      int64(len(child)),
	}
	if err := src.Push(ctx, childDesc, bytes.NewReader(child)); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d}]}`,
		ocispec.MediaTypeImageIndex, childDesc.MediaType, childDesc.Digest, childDesc.Size))
	indexDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(index),
		Size:      int64(len(index)),
	}
	if err := src.Push(ctx, indexDesc, bytes.NewReader(index)); err != nil {
		t.Fatal(err)
	}

	opts := oras.DefaultExtendedCopyGraphOptions
	opts.FindPredecessors = func(_ context.Context, _ content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if content.Equal(desc, indexDesc) {
			return []ocispec.Descriptor{childDesc}, nil
		}
		return nil, nil
	}
	return src, indexDesc, opts
}

// Test_recursiveCopy_strandedRoot covers the copy failure reported in
// https://github.com/oras-project/oras/issues/2148. Extended copy walks up to
// the bogus referrers, treats the child as the graph root and never copies the
// index, which used to leave the final tagging with nothing to tag.
func Test_recursiveCopy_strandedRoot(t *testing.T) {
	ctx := context.Background()
	src, indexDesc, opts := newStrandedRootSource(t)
	dst := memory.New()

	if err := recursiveCopy(ctx, src, dst, "v1", indexDesc, opts); err != nil {
		t.Fatalf("recursiveCopy() error = %v, wantErr false", err)
	}

	exists, err := dst.Exists(ctx, indexDesc)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("recursiveCopy() left the root index out of the destination")
	}
	got, err := dst.Resolve(ctx, "v1")
	if err != nil {
		t.Fatalf("Resolve() error = %v, wantErr false", err)
	}
	if !content.Equal(got, indexDesc) {
		t.Errorf("Resolve() = %v, want %v", got, indexDesc)
	}
}

var errMockedPush = errors.New("push error")

// pushFailingTarget is a mock implementation of oras.Target that rejects one
// descriptor, simulating a destination that cannot accept the root.
type pushFailingTarget struct {
	oras.Target
	failOn ocispec.Descriptor
}

// Push simulates a failure for the tracked descriptor.
func (t *pushFailingTarget) Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error {
	if content.Equal(desc, t.failOn) {
		return errMockedPush
	}
	return t.Target.Push(ctx, desc, r)
}

func Test_recursiveCopy_rootPushFailure(t *testing.T) {
	ctx := context.Background()
	src, indexDesc, opts := newStrandedRootSource(t)
	dst := &pushFailingTarget{Target: memory.New(), failOn: indexDesc}

	err := recursiveCopy(ctx, src, dst, "v1", indexDesc, opts)
	if !errors.Is(err, errMockedPush) {
		t.Fatalf("recursiveCopy() error = %v, want to wrap %v", err, errMockedPush)
	}
	var copyErr *oras.CopyError
	if !errors.As(err, &copyErr) {
		t.Fatalf("recursiveCopy() error = %v, want *oras.CopyError", err)
	}
	if copyErr.Op != "Push" {
		t.Errorf("recursiveCopy() error Op = %q, want %q", copyErr.Op, "Push")
	}
	if copyErr.Origin != oras.CopyErrorOriginDestination {
		t.Errorf("recursiveCopy() error Origin = %v, want %v", copyErr.Origin, oras.CopyErrorOriginDestination)
	}
}

// Test_recursiveCopy_genuineRootReferrerSurvives guards the filter above
// against over-filtering: a real referrer of the root must still be copied
// even when the same FindPredecessors call also reports the index's own
// children as referrers.
func Test_recursiveCopy_genuineRootReferrerSurvives(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	push := func(blob []byte, mediaType string) ocispec.Descriptor {
		desc := ocispec.Descriptor{
			MediaType: mediaType,
			Digest:    digest.FromBytes(blob),
			Size:      int64(len(blob)),
		}
		if err := src.Push(ctx, desc, bytes.NewReader(blob)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	configDesc := push(configContent, configMediaType)
	childDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[]}`,
		ocispec.MediaTypeImageManifest, configDesc.MediaType, configDesc.Digest, configDesc.Size)), ocispec.MediaTypeImageManifest)
	indexDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d}]}`,
		ocispec.MediaTypeImageIndex, childDesc.MediaType, childDesc.Digest, childDesc.Size)), ocispec.MediaTypeImageIndex)
	// A genuine referrer of the index, i.e. a manifest declaring it as subject.
	referrerDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"artifactType":"application/vnd.test.referrer","config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[],"subject":{"mediaType":%q,"digest":%q,"size":%d}}`,
		ocispec.MediaTypeImageManifest, configDesc.MediaType, configDesc.Digest, configDesc.Size,
		indexDesc.MediaType, indexDesc.Digest, indexDesc.Size)), ocispec.MediaTypeImageManifest)

	opts := oras.DefaultExtendedCopyGraphOptions
	opts.FindPredecessors = func(_ context.Context, _ content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if content.Equal(desc, indexDesc) {
			// The phantom child and the genuine referrer, reported together.
			return []ocispec.Descriptor{childDesc, referrerDesc}, nil
		}
		return nil, nil
	}

	dst := memory.New()
	if err := recursiveCopy(ctx, src, dst, "v1", indexDesc, opts); err != nil {
		t.Fatalf("recursiveCopy() error = %v, wantErr false", err)
	}
	for name, desc := range map[string]ocispec.Descriptor{
		"index":    indexDesc,
		"child":    childDesc,
		"referrer": referrerDesc,
	} {
		exists, err := dst.Exists(ctx, desc)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("recursiveCopy() left the %s out of the destination", name)
		}
	}
}

// Test_recursiveCopy_strandedRootWithChildReferrer covers the #1728 shape of
// https://github.com/oras-project/oras/issues/2148: the index has no referrer
// of its own, but a child does, so FindPredecessors reports the phantom
// referrers on the path where the child referrer list is non-empty.
func Test_recursiveCopy_strandedRootWithChildReferrer(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	push := func(blob []byte, mediaType string) ocispec.Descriptor {
		desc := ocispec.Descriptor{
			MediaType: mediaType,
			Digest:    digest.FromBytes(blob),
			Size:      int64(len(blob)),
		}
		if err := src.Push(ctx, desc, bytes.NewReader(blob)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	configDesc := push(configContent, configMediaType)
	childDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[]}`,
		ocispec.MediaTypeImageManifest, configDesc.MediaType, configDesc.Digest, configDesc.Size)), ocispec.MediaTypeImageManifest)
	indexDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d}]}`,
		ocispec.MediaTypeImageIndex, childDesc.MediaType, childDesc.Digest, childDesc.Size)), ocispec.MediaTypeImageIndex)
	// A genuine referrer of the child, not of the index.
	referrerDesc := push([]byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"artifactType":"application/vnd.test.referrer","config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[],"subject":{"mediaType":%q,"digest":%q,"size":%d}}`,
		ocispec.MediaTypeImageManifest, configDesc.MediaType, configDesc.Digest, configDesc.Size,
		childDesc.MediaType, childDesc.Digest, childDesc.Size)), ocispec.MediaTypeImageManifest)

	opts := oras.DefaultExtendedCopyGraphOptions
	opts.FindPredecessors = func(_ context.Context, _ content.ReadOnlyGraphStorage, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		switch {
		case content.Equal(desc, indexDesc):
			// Phantom: the index's own child, reported as its referrer.
			return []ocispec.Descriptor{childDesc}, nil
		case content.Equal(desc, childDesc):
			return []ocispec.Descriptor{referrerDesc}, nil
		}
		return nil, nil
	}

	dst := memory.New()
	if err := recursiveCopy(ctx, src, dst, "v1", indexDesc, opts); err != nil {
		t.Fatalf("recursiveCopy() error = %v, wantErr false", err)
	}
	for name, desc := range map[string]ocispec.Descriptor{
		"index":    indexDesc,
		"child":    childDesc,
		"referrer": referrerDesc,
	} {
		exists, err := dst.Exists(ctx, desc)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("recursiveCopy() left the %s out of the destination", name)
		}
	}
}

func newMultiPlatformSource(t *testing.T) (*memory.Store, ocispec.Descriptor, ocispec.Index) {
	t.Helper()
	ctx := context.Background()
	src := memory.New()
	config := []byte(`{}`)
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(config),
		Size:      int64(len(config)),
	}
	if err := src.Push(ctx, configDesc, bytes.NewReader(config)); err != nil {
		t.Fatal(err)
	}

	platforms := []*ocispec.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm", Variant: "v6"},
		{OS: "linux", Architecture: "arm", Variant: "v7"},
	}
	manifests := make([]ocispec.Descriptor, 0, len(platforms))
	for _, platform := range platforms {
		manifest := ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Annotations: map[string]string{
				"test.platform": formatPlatform(platform),
			},
			Config: configDesc,
			Layers: []ocispec.Descriptor{},
		}
		contentBytes, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		desc := ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    digest.FromBytes(contentBytes),
			Size:      int64(len(contentBytes)),
			Platform:  platform,
		}
		if err := src.Push(ctx, desc, bytes.NewReader(contentBytes)); err != nil {
			t.Fatal(err)
		}
		manifests = append(manifests, desc)
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: manifests,
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	if err := src.Push(ctx, root, bytes.NewReader(indexBytes)); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, root, "source"); err != nil {
		t.Fatal(err)
	}
	return src, root, index
}

func newBuildxMultiPlatformSource(t *testing.T) (*memory.Store, ocispec.Descriptor, ocispec.Index) {
	t.Helper()
	ctx := context.Background()
	src, _, index := newMultiPlatformSource(t)
	for _, manifest := range index.Manifests {
		attestation := ocispec.Manifest{
			Versioned:    specs.Versioned{SchemaVersion: 2},
			MediaType:    ocispec.MediaTypeImageManifest,
			ArtifactType: "application/vnd.in-toto+json",
			Config: ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageConfig,
				Digest:    digest.FromBytes([]byte(`{}`)),
				Size:      2,
			},
			Layers: []ocispec.Descriptor{},
		}
		attestation.Annotations = map[string]string{"test.subject": manifest.Digest.String()}
		contentBytes, err := json.Marshal(attestation)
		if err != nil {
			t.Fatal(err)
		}
		desc := ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    digest.FromBytes(contentBytes),
			Size:      int64(len(contentBytes)),
			Platform:  &ocispec.Platform{OS: "unknown", Architecture: "unknown"},
			Annotations: map[string]string{
				dockerReferenceDigestAnnotation: manifest.Digest.String(),
			},
		}
		if err := src.Push(ctx, desc, bytes.NewReader(contentBytes)); err != nil {
			t.Fatal(err)
		}
		index.Manifests = append(index.Manifests, desc)
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromBytes(indexBytes),
		Size:      int64(len(indexBytes)),
	}
	if err := src.Push(ctx, root, bytes.NewReader(indexBytes)); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, root, "buildx-source"); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, root, root.Digest.String()); err != nil {
		t.Fatal(err)
	}
	return src, root, index
}

type fetchCountingTarget struct {
	oras.ReadOnlyGraphTarget
	root        ocispec.Descriptor
	rootFetches int
}

type fetchOverrideTarget struct {
	oras.ReadOnlyGraphTarget
	root ocispec.Descriptor
	data []byte
	err  error
}

func (s *fetchOverrideTarget) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	if reference == "source" {
		return s.root, nil
	}
	return s.ReadOnlyGraphTarget.Resolve(ctx, reference)
}

func (s *fetchOverrideTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if content.Equal(desc, s.root) {
		if s.err != nil {
			return nil, s.err
		}
		return io.NopCloser(bytes.NewReader(s.data)), nil
	}
	return s.ReadOnlyGraphTarget.Fetch(ctx, desc)
}

func (s *fetchCountingTarget) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if content.Equal(desc, s.root) {
		s.rootFetches++
	}
	return s.ReadOnlyGraphTarget.Fetch(ctx, desc)
}

type discardCopyHandler struct {
	status.DiscardHandler
}

func (discardCopyHandler) OnMounted(context.Context, ocispec.Descriptor) error {
	return nil
}

type discardMetadataHandler struct {
	metadata.Discard
}

func (discardMetadataHandler) OnCopied(*option.BinaryTarget, ocispec.Descriptor) error {
	return nil
}

func Test_filterManifestsByPlatform(t *testing.T) {
	src, root, _ := newMultiPlatformSource(t)
	platforms := option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "arm"}}}
	opts := &copyOptions{Platforms: platforms}
	_, index, selected, err := filterManifestByPlatform(context.Background(), src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected %d manifests, want 2", len(selected))
	}

	opts.Platforms.Platforms = []*ocispec.Platform{{OS: "linux", Architecture: "arm", Variant: "v7"}}
	_, _, selected, err = filterManifestByPlatform(context.Background(), src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || index.Manifests[selected[0]].Platform.Variant != "v7" {
		t.Fatalf("selected %#v, want only arm/v7", selected)
	}

	opts.Platforms.Platforms = []*ocispec.Platform{{OS: "windows", Architecture: "amd64"}}
	if _, _, _, err = filterManifestByPlatform(context.Background(), src, root, opts); err == nil || !strings.Contains(err.Error(), "linux/amd64") {
		t.Fatalf("filterManifestsByPlatform() error = %v, want available platforms", err)
	}
}

func Test_filterManifestByPlatform_attributesSourceFailures(t *testing.T) {
	root := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageIndex,
		Digest:    digest.FromString("source-index"),
		Size:      12,
	}
	opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}}}
	tests := []struct {
		name string
		src  oras.ReadOnlyGraphTarget
	}{
		{name: "fetch", src: &fetchFailingReadOnlyGraphTarget{}},
		{name: "parse", src: &invalidJSONReadOnlyGraphTarget{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := filterManifestByPlatform(context.Background(), tt.src, root, opts)
			var copyErr *oras.CopyError
			if !errors.As(err, &copyErr) {
				t.Fatalf("filterManifestByPlatform() error = %v, want *oras.CopyError", err)
			}
			if copyErr.Origin != oras.CopyErrorOriginSource {
				t.Fatalf("copy error origin = %v, want source", copyErr.Origin)
			}
		})
	}
}

func Test_filterManifestByPlatform_recommendsUniqueImagePlatforms(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	amd64 := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("amd64"),
		Size:      10,
		Platform:  &ocispec.Platform{OS: "linux", Architecture: "amd64"},
	}
	attestation := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("attestation"),
		Size:      10,
		Platform:  &ocispec.Platform{OS: "unknown", Architecture: "unknown"},
		Annotations: map[string]string{
			dockerReferenceDigestAnnotation: amd64.Digest.String(),
		},
	}
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{amd64, amd64, attestation},
	}
	indexContent, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
	if err := src.Push(ctx, root, bytes.NewReader(indexContent)); err != nil {
		t.Fatal(err)
	}
	opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "windows", Architecture: "amd64"}}}}

	_, _, _, err = filterManifestByPlatform(ctx, src, root, opts)
	if err == nil {
		t.Fatal("filterManifestByPlatform() error = nil, want unmatched platform error")
	}
	if got := err.Error(); !strings.Contains(got, "available platforms in index: [linux/amd64]") || strings.Contains(got, "unknown/unknown") {
		t.Fatalf("filterManifestByPlatform() error = %q, want one image platform and no attestation platform", got)
	}
}

func Test_filterManifestByPlatform_selectsSharedDigestByPosition(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	sharedDigest := digest.FromString("shared-arm-manifest")
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{
			{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("amd64"), Size: 10, Platform: &ocispec.Platform{OS: "linux", Architecture: "amd64"}},
			{MediaType: ocispec.MediaTypeImageManifest, Digest: sharedDigest, Size: 20, Platform: &ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v6"}},
			{MediaType: ocispec.MediaTypeImageManifest, Digest: sharedDigest, Size: 20, Platform: &ocispec.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}},
		},
	}
	indexContent, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
	if err := src.Push(ctx, root, bytes.NewReader(indexContent)); err != nil {
		t.Fatal(err)
	}
	opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm", Variant: "v7"},
	}}}

	_, _, kept, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(kept, []int{0, 2}) {
		t.Fatalf("kept positions = %v, want [0 2]", kept)
	}
}

func Test_platformMatches(t *testing.T) {
	manifestPlatform := &ocispec.Platform{OS: "windows", Architecture: "amd64", Variant: "v2", OSVersion: "10.0"}
	tests := []struct {
		name     string
		manifest *ocispec.Platform
		target   *ocispec.Platform
		want     bool
	}{
		{name: "nil manifest", target: manifestPlatform},
		{name: "nil target", manifest: manifestPlatform},
		{name: "OS mismatch", manifest: &ocispec.Platform{OS: "linux", Architecture: "amd64"}, target: manifestPlatform},
		{name: "architecture mismatch", manifest: &ocispec.Platform{OS: "windows", Architecture: "arm64"}, target: manifestPlatform},
		{name: "variant mismatch", manifest: &ocispec.Platform{OS: "windows", Architecture: "amd64", Variant: "v1"}, target: manifestPlatform},
		{name: "OS version mismatch", manifest: &ocispec.Platform{OS: "windows", Architecture: "amd64", OSVersion: "11"}, target: manifestPlatform},
		{name: "omitted optional fields match", manifest: manifestPlatform, target: &ocispec.Platform{OS: "windows", Architecture: "amd64"}, want: true},
		{name: "all fields match", manifest: manifestPlatform, target: manifestPlatform, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := platformMatches(tt.manifest, tt.target); got != tt.want {
				t.Fatalf("platformMatches(%#v, %#v) = %t, want %t", tt.manifest, tt.target, got, tt.want)
			}
		})
	}
}

func Test_formatPlatform(t *testing.T) {
	tests := []struct {
		name string
		in   *ocispec.Platform
		want string
	}{
		{name: "unknown", in: nil, want: "<unknown>"},
		{name: "basic", in: &ocispec.Platform{OS: "linux", Architecture: "amd64"}, want: "linux/amd64"},
		{name: "variant and OS version", in: &ocispec.Platform{OS: "windows", Architecture: "amd64", Variant: "v2", OSVersion: "10.0"}, want: "windows/amd64/v2:10.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPlatform(tt.in); got != tt.want {
				t.Fatalf("formatPlatform(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func Test_keepsManifest(t *testing.T) {
	selected := ocispec.Descriptor{Digest: digest.FromString("selected"), Platform: &ocispec.Platform{OS: "linux", Architecture: "amd64"}}
	tests := []struct {
		name              string
		manifest          ocispec.Descriptor
		unselectedSibling *ocispec.Descriptor
		selected          map[int]bool
		want              bool
	}{
		{name: "platformless entry is retained", manifest: ocispec.Descriptor{Digest: digest.FromString("unknown")}, want: true},
		{name: "platformless alias of an unselected manifest is dropped", manifest: ocispec.Descriptor{Digest: digest.FromString("unselected")}, unselectedSibling: &ocispec.Descriptor{Digest: digest.FromString("unselected"), Platform: &ocispec.Platform{OS: "linux", Architecture: "arm64"}}, selected: map[int]bool{0: true}, want: false},
		{name: "selected buildx attestation is retained", manifest: ocispec.Descriptor{Annotations: map[string]string{dockerReferenceDigestAnnotation: selected.Digest.String()}}, selected: map[int]bool{0: true}, want: true},
		{name: "unselected buildx attestation is dropped", manifest: ocispec.Descriptor{Annotations: map[string]string{dockerReferenceDigestAnnotation: digest.FromString("other").String()}}, want: false},
		{name: "unselected platform is dropped", manifest: ocispec.Descriptor{Platform: &ocispec.Platform{OS: "linux", Architecture: "arm64"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifests := []ocispec.Descriptor{selected}
			if tt.unselectedSibling != nil {
				manifests = append(manifests, *tt.unselectedSibling)
			}
			manifests = append(manifests, tt.manifest)
			if got := keepsManifest(len(manifests)-1, tt.manifest, manifests, tt.selected); got != tt.want {
				t.Fatalf("keepsManifest(%#v) = %t, want %t", tt.manifest, got, tt.want)
			}
		})
	}
}

func Test_doMultipleCopy_copiesFilteredIndex(t *testing.T) {
	ctx := context.Background()
	src, root, index := newMultiPlatformSource(t)
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm", Variant: "v7"},
		}},
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	indexContent, _, selected, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := doMultipleCopy(ctx, logrus.New(), statusHandler, metadataHandler, src, dst, opts, root, indexContent, selected); err != nil {
		t.Fatal(err)
	}

	gotRoot, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if content.Equal(gotRoot, root) {
		t.Fatal("filtered copy retained the original root digest")
	}
	filteredBytes, err := content.FetchAll(ctx, dst, gotRoot)
	if err != nil {
		t.Fatal(err)
	}
	var filtered ocispec.Index
	if err := json.Unmarshal(filteredBytes, &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Manifests) != 2 {
		t.Fatalf("filtered index contains %d manifests, want 2", len(filtered.Manifests))
	}
	if filtered.Manifests[1].Platform.Variant != "v7" {
		t.Fatalf("filtered index contains %#v, want arm/v7", filtered.Manifests[1].Platform)
	}
	if exists, err := dst.Exists(ctx, index.Manifests[1]); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Fatal("filtered copy included the unselected arm/v6 manifest")
	}
}

func Test_copyMultiplePlatforms_allSelectionPreservesRoot(t *testing.T) {
	ctx := context.Background()
	src, root, _ := newMultiPlatformSource(t)
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm"},
		}},
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := copyMultiplePlatforms(ctx, logrus.New(), statusHandler, metadataHandler, src, dst, opts); err != nil {
		t.Fatal(err)
	}
	got, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if !content.Equal(got, root) {
		t.Fatalf("destination root = %v, want original root %v", got.Digest, root.Digest)
	}
}

func Test_copyMultiplePlatforms_equivalentSelectorsCopyManifest(t *testing.T) {
	ctx := context.Background()
	src, _, index := newMultiPlatformSource(t)
	index.Manifests = []ocispec.Descriptor{index.Manifests[0], index.Manifests[2]}
	indexContent, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
	if err := src.Push(ctx, root, bytes.NewReader(indexContent)); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, root, "source"); err != nil {
		t.Fatal(err)
	}
	dst := memory.New()
	opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{
		{OS: "linux", Architecture: "arm"},
		{OS: "linux", Architecture: "arm", Variant: "v7"},
	}}}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := copyMultiplePlatforms(ctx, logrus.New(), statusHandler, metadataHandler, src, dst, opts); err != nil {
		t.Fatal(err)
	}
	got, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if got.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("destination media type = %q, want image manifest", got.MediaType)
	}
	if got.Digest != index.Manifests[1].Digest {
		t.Fatalf("destination digest = %v, want selected arm/v7 manifest %v", got.Digest, index.Manifests[1].Digest)
	}
}

func Test_copyMultiplePlatforms_buildxAttestationsPreserveRootAndSourceReference(t *testing.T) {
	ctx := context.Background()
	src, root, index := newBuildxMultiPlatformSource(t)
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm", Variant: "v6"},
			{OS: "linux", Architecture: "arm", Variant: "v7"},
		}},
	}
	opts.From.Reference = root.Digest.String()
	opts.From.Path = "registry.example/image"
	opts.From.RawReference = opts.From.Path + "@" + root.Digest.String()
	opts.To.Reference = "destination"
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := copyMultiplePlatforms(ctx, logrus.New(), statusHandler, metadataHandler, src, dst, opts); err != nil {
		t.Fatal(err)
	}
	got, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if !content.Equal(got, root) {
		t.Fatalf("destination root = %v, want original root %v", got.Digest, root.Digest)
	}
	if opts.From.RawReference != "registry.example/image@"+root.Digest.String() {
		t.Fatalf("source reference = %q, want original source digest", opts.From.RawReference)
	}
	if len(index.Manifests) != 6 {
		t.Fatalf("buildx index has %d entries, want 6", len(index.Manifests))
	}
}

func Test_copyMultiplePlatforms_buildxSubsetKeepsOnlySelectedAttestations(t *testing.T) {
	ctx := context.Background()
	src, root, _ := newBuildxMultiPlatformSource(t)
	countingSrc := &fetchCountingTarget{ReadOnlyGraphTarget: src, root: root}
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm", Variant: "v7"},
		}},
	}
	opts.From.Reference = root.Digest.String()
	opts.From.Path = "registry.example/image"
	opts.From.RawReference = opts.From.Path + "@" + root.Digest.String()
	opts.To.Reference = "destination"
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := copyMultiplePlatforms(ctx, logrus.New(), statusHandler, metadataHandler, countingSrc, dst, opts); err != nil {
		t.Fatal(err)
	}
	if opts.From.RawReference != "registry.example/image@"+root.Digest.String() {
		t.Fatalf("source reference = %q, want original source digest", opts.From.RawReference)
	}
	if countingSrc.rootFetches != 1 {
		t.Fatalf("source index fetched %d times, want once", countingSrc.rootFetches)
	}
	gotRoot, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if content.Equal(gotRoot, root) {
		t.Fatal("subset copy retained the original index digest")
	}
	filteredBytes, err := content.FetchAll(ctx, dst, gotRoot)
	if err != nil {
		t.Fatal(err)
	}
	var filtered ocispec.Index
	if err := json.Unmarshal(filteredBytes, &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Manifests) != 4 {
		t.Fatalf("filtered index has %d entries, want two platforms and two attestations", len(filtered.Manifests))
	}
	for _, manifest := range filtered.Manifests {
		if manifest.Platform != nil && manifest.Platform.OS == "unknown" && manifest.Annotations[dockerReferenceDigestAnnotation] == "" {
			t.Fatal("retained buildx attestation has no subject reference")
		}
		if manifest.Platform != nil && manifest.Platform.Architecture == "arm" && manifest.Platform.Variant == "v6" {
			t.Fatal("filtered index contains unselected arm/v6 manifest")
		}
	}
}

func Test_doMultipleCopy_recursiveCopiesSelectedReferrer(t *testing.T) {
	ctx := context.Background()
	src, root, index := newMultiPlatformSource(t)
	referrer, err := oras.PackManifest(ctx, src, oras.PackManifestVersion1_1, "application/vnd.test.signature", oras.PackManifestOptions{Subject: &index.Manifests[0]})
	if err != nil {
		t.Fatal(err)
	}
	rootReferrer, err := oras.PackManifest(ctx, src, oras.PackManifestVersion1_1, "application/vnd.test.index-signature", oras.PackManifestOptions{Subject: &root})
	if err != nil {
		t.Fatal(err)
	}
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}},
		recursive: true,
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	var warning bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&warning)
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	indexContent, _, selected, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := doMultipleCopy(ctx, logger, statusHandler, metadataHandler, src, dst, opts, root, indexContent, selected); err != nil {
		t.Fatal(err)
	}
	if exists, err := dst.Exists(ctx, referrer); err != nil {
		t.Fatal(err)
	} else if !exists {
		t.Fatal("recursive filtered copy did not include the selected manifest referrer")
	}
	if exists, err := dst.Exists(ctx, rootReferrer); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Fatal("recursive filtered copy included a referrer of the original index")
	}
	if exists, err := dst.Exists(ctx, index.Manifests[1]); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Fatal("recursive filtered copy included an unselected manifest")
	}
	if got := warning.String(); !strings.Contains(got, "level=warning") || !strings.Contains(got, "referrers of the source index are not copied") {
		t.Fatalf("warning = %q, want source-index referrer warning", got)
	}
}

func Test_doMultipleCopy_recursiveDoesNotWarnWithoutRootReferrers(t *testing.T) {
	ctx := context.Background()
	src, root, _ := newMultiPlatformSource(t)
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}},
		recursive: true,
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	var warning bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&warning)
	indexContent, _, selected, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := doMultipleCopy(ctx, logger, statusHandler, metadataHandler, src, dst, opts, root, indexContent, selected); err != nil {
		t.Fatal(err)
	}
	if got := warning.String(); got != "" {
		t.Fatalf("warning = %q, want no warning for a referrer-free source index", got)
	}
}

type rootReferrersFailingSource struct {
	oras.ReadOnlyGraphTarget
	rootDigest digest.Digest
	err        error
}

func (s *rootReferrersFailingSource) Referrers(_ context.Context, desc ocispec.Descriptor, _ string, fn func([]ocispec.Descriptor) error) error {
	if desc.Digest == s.rootDigest {
		return s.err
	}
	return fn(nil)
}

func Test_doMultipleCopy_recursiveContinuesWhenRootReferrerProbeFails(t *testing.T) {
	ctx := context.Background()
	base, root, _ := newMultiPlatformSource(t)
	src := &rootReferrersFailingSource{ReadOnlyGraphTarget: base, rootDigest: root.Digest, err: errors.New("referrer lookup unavailable")}
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}},
		recursive: true,
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	indexContent, _, selected, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	root.Annotations = map[string]string{"stale": "annotation"}
	root.Data = []byte("stale descriptor data")
	var warning bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&warning)
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := doMultipleCopy(ctx, logger, statusHandler, metadataHandler, src, dst, opts, root, indexContent, selected); err != nil {
		t.Fatalf("doMultipleCopy() error = %v, want copy to continue after warning probe failure", err)
	}
	if got := warning.String(); !strings.Contains(got, "level=warning") || !strings.Contains(got, "referrers of the source index are not copied") {
		t.Fatalf("warning = %q, want source-index referrer warning after probe failure", got)
	}
	gotRoot, err := dst.Resolve(ctx, "destination")
	if err != nil {
		t.Fatalf("destination was not copied: %v", err)
	}
	if gotRoot.Annotations != nil || len(gotRoot.Data) != 0 {
		t.Fatalf("filtered root retained source descriptor metadata: %#v", gotRoot)
	}
}

type testReferrerSource struct {
	*memory.Store
	called bool
}

func (s *testReferrerSource) Referrers(_ context.Context, _ ocispec.Descriptor, _ string, fn func([]ocispec.Descriptor) error) error {
	s.called = true
	return fn(nil)
}

func Test_filteredIndexReferrerSource(t *testing.T) {
	root := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex, Digest: digest.FromString("root"), Size: 4}
	underlying := &testReferrerSource{Store: memory.New()}
	base := &filteredIndexSource{ReadOnlyGraphTarget: underlying, root: root}
	wrapped := &filteredIndexReferrerSource{filteredIndexSource: base, ReferrerLister: underlying}

	if got := wrapped.Unwrap(); got != underlying {
		t.Fatal("Unwrap() did not return the underlying source")
	}
	if err := wrapped.Referrers(context.Background(), root, "", func([]ocispec.Descriptor) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if underlying.called {
		t.Fatal("root referrers should be suppressed for a filtered root")
	}
	other := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("other"), Size: 5}
	if err := wrapped.Referrers(context.Background(), other, "", func([]ocispec.Descriptor) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !underlying.called {
		t.Fatal("underlying Referrers API was not used for a child")
	}
}

func Test_filteredIndexSource_delegatesNonRootOperations(t *testing.T) {
	ctx := context.Background()
	underlying := memory.New()
	childContent := []byte("child")
	child := content.NewDescriptorFromBytes("application/octet-stream", childContent)
	if err := underlying.Push(ctx, child, bytes.NewReader(childContent)); err != nil {
		t.Fatal(err)
	}
	if err := underlying.Tag(ctx, child, "child"); err != nil {
		t.Fatal(err)
	}
	rootContent := []byte(`{"schemaVersion":2,"manifests":[]}`)
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, rootContent)
	source := &filteredIndexSource{
		ReadOnlyGraphTarget: underlying,
		reference:           "source",
		root:                root,
		content:             rootContent,
	}

	if got, err := source.Resolve(ctx, "source"); err != nil || !content.Equal(got, root) {
		t.Fatalf("Resolve(source) = %v, %v, want %v, nil", got, err, root)
	}
	if got, err := source.Resolve(ctx, "child"); err != nil || !content.Equal(got, child) {
		t.Fatalf("Resolve(child) = %v, %v, want %v, nil", got, err, child)
	}
	reader, err := source.Fetch(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	gotRootContent, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(gotRootContent, rootContent) {
		t.Fatalf("Fetch(root) = %q, %v, want %q, nil", gotRootContent, err, rootContent)
	}
	reader, err = source.Fetch(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	gotChildContent, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(gotChildContent, childContent) {
		t.Fatalf("Fetch(child) = %q, %v, want %q, nil", gotChildContent, err, childContent)
	}
	for _, target := range []ocispec.Descriptor{root, child} {
		if exists, err := source.Exists(ctx, target); err != nil || !exists {
			t.Fatalf("Exists(%v) = %v, %v, want true, nil", target.Digest, exists, err)
		}
	}
	if exists, err := source.Exists(ctx, ocispec.Descriptor{Digest: digest.FromString("missing")}); err != nil || exists {
		t.Fatalf("Exists(missing) = %v, %v, want false, nil", exists, err)
	}
	if predecessors, err := source.Predecessors(ctx, root); err != nil || predecessors != nil {
		t.Fatalf("Predecessors(root) = %v, %v, want nil, nil", predecessors, err)
	}
	if predecessors, err := source.Predecessors(ctx, child); err != nil || predecessors != nil {
		t.Fatalf("Predecessors(child) = %v, %v, want nil, nil", predecessors, err)
	}
}

func Test_doMultipleCopy_usesReferrerListerWrapper(t *testing.T) {
	ctx := context.Background()
	src := &testReferrerSource{Store: memory.New()}
	selectedContent := []byte("selected")
	unselectedContent := []byte("unselected")
	selected := content.NewDescriptorFromBytes("application/octet-stream", selectedContent)
	unselected := content.NewDescriptorFromBytes("application/octet-stream", unselectedContent)
	selected.Platform = &ocispec.Platform{OS: "linux", Architecture: "amd64"}
	unselected.Platform = &ocispec.Platform{OS: "linux", Architecture: "arm64"}
	for _, item := range []struct {
		desc ocispec.Descriptor
		data []byte
	}{
		{selected, selectedContent},
		{unselected, unselectedContent},
	} {
		if err := src.Push(ctx, item.desc, bytes.NewReader(item.data)); err != nil {
			t.Fatal(err)
		}
	}
	index := ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: []ocispec.Descriptor{selected, unselected}}
	indexContent, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
	if err := src.Push(ctx, root, bytes.NewReader(indexContent)); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, root, "source"); err != nil {
		t.Fatal(err)
	}
	dst := memory.New()
	opts := &copyOptions{
		Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}},
	}
	opts.From.Reference = "source"
	opts.To.Reference = "destination"
	statusHandler := discardCopyHandler{DiscardHandler: status.NewDiscardHandler()}
	metadataHandler := discardMetadataHandler{Discard: metadata.NewDiscardHandler()}
	if err := doMultipleCopy(ctx, logrus.New(), statusHandler, metadataHandler, src, dst, opts, root, indexContent, []int{0}); err != nil {
		t.Fatal(err)
	}
	if got, err := dst.Resolve(ctx, "destination"); err != nil {
		t.Fatal(err)
	} else if got.Digest == root.Digest {
		t.Fatal("filtered copy retained the original root digest")
	}
}

func Test_filterIndexContent_preservesExtensions(t *testing.T) {
	indexContent := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","x-vendor":{"future":true},"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":10,"platform":{"os":"linux","architecture":"amd64","features":["sse4"]},"x-descriptor":"keep"},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","size":11,"platform":{"os":"linux","architecture":"arm64"}}]}`)
	filtered, err := filterIndexContent(indexContent, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(filtered, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["x-vendor"]; !ok {
		t.Fatal("filtered index dropped the top-level extension")
	}
	if !strings.Contains(string(filtered), `"features":["sse4"]`) || !strings.Contains(string(filtered), `"x-descriptor":"keep"`) {
		t.Fatalf("filtered index dropped descriptor extensions: %s", filtered)
	}
	keys := []string{`"schemaVersion"`, `"mediaType"`, `"x-vendor"`, `"manifests"`}
	last := -1
	for _, key := range keys {
		position := strings.Index(string(filtered), key)
		if position <= last {
			t.Fatalf("filtered index changed the source field order for %s: %s", key, filtered)
		}
		last = position
	}
	var manifests []json.RawMessage
	if err := json.Unmarshal(raw["manifests"], &manifests); err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 {
		t.Fatalf("filtered manifest count = %d, want 1", len(manifests))
	}
}

func Test_filterIndexContent_errors(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "invalid index", content: []byte("{")},
		{name: "non-object index", content: []byte(`[]`)},
		{name: "missing manifests", content: []byte(`{"schemaVersion":2}`)},
		{name: "invalid manifests", content: []byte(`{"manifests":{}}`)},
		{name: "invalid descriptor", content: []byte(`{"manifests":[1]}`)},
		{name: "trailing index value", content: []byte(`{"manifests":[]} {}`)},
		{name: "trailing manifests value", content: []byte(`{"manifests":[] []}`)},
		{name: "duplicate manifests arrays", content: []byte(`{"manifests":[],"manifests":[]}`)},
		{name: "truncated index object", content: []byte(`{"manifests":[]`)},
		{name: "malformed index value", content: []byte(`{"manifests":[],"extension":invalid}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := filterIndexContent(tt.content, nil); err == nil {
				t.Fatal("filterIndexContent() error = nil, want an error")
			}
		})
	}
}

func Test_filterManifestArray_errors(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "not an array", content: []byte(`{}`)},
		{name: "invalid descriptor", content: []byte(`[1]`)},
		{name: "invalid array value", content: []byte(`[{},]`)},
		{name: "truncated array", content: []byte(`[{} `)},
		{name: "trailing JSON value", content: []byte(`[{}] {}`)},
		{name: "malformed trailing data", content: []byte(`[{}] x`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := filterManifestArray(tt.content, nil); err == nil {
				t.Fatal("filterManifestArray() error = nil, want an error")
			}
		})
	}
}

func Test_filterManifestByPlatform_keepsAssociatedDescriptors(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	selected := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("selected"),
		Size:      8,
		Platform:  &ocispec.Platform{OS: "linux", Architecture: "amd64"},
	}
	associated := ocispec.Descriptor{
		MediaType:   "application/vnd.in-toto+json",
		Digest:      digest.FromString("attestation"),
		Size:        10,
		Annotations: map[string]string{"vnd.docker.reference.digest": selected.Digest.String()},
	}
	unselected := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromString("unselected"),
		Size:      10,
		Platform:  &ocispec.Platform{OS: "linux", Architecture: "arm64"},
	}
	index := ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: []ocispec.Descriptor{selected, associated, unselected}}
	indexContent, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
	if err := src.Push(ctx, root, bytes.NewReader(indexContent)); err != nil {
		t.Fatal(err)
	}
	opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "linux", Architecture: "amd64"}}}}
	_, _, filtered, err := filterManifestByPlatform(ctx, src, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0] != 0 || filtered[1] != 1 {
		t.Fatalf("filtered manifest positions = %#v, want [0 1]", filtered)
	}
}

func Test_copyMultiplePlatforms_plainManifestError(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestContent)
	if err := src.Push(ctx, desc, bytes.NewReader(manifestContent)); err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(ctx, desc, "plain"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name          string
		rawReference  string
		wantReference string
	}{
		{name: "preserves input reference", rawReference: "src-layout:plain", wantReference: `"src-layout:plain"`},
		{name: "falls back to parsed reference", wantReference: `"plain"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := &copyOptions{}
			opts.From.RawReference = tt.rawReference
			opts.From.Reference = "plain"
			err := copyMultiplePlatforms(ctx, logrus.New(), nil, nil, src, memory.New(), opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantReference+" is not an image index or a manifest list") {
				t.Fatalf("copyMultiplePlatforms() error = %v, want source reference %s", err, tt.wantReference)
			}
			if !strings.Contains(err.Error(), "use a single --platform value") {
				t.Fatalf("copyMultiplePlatforms() error = %v, want recommendation", err)
			}
		})
	}
}

func Test_copyMultiplePlatforms_reportsIndexErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("resolve failure", func(t *testing.T) {
		opts := &copyOptions{}
		opts.From.Reference = "missing"
		err := copyMultiplePlatforms(ctx, logrus.New(), nil, nil, memory.New(), memory.New(), opts)
		var copyErr *oras.CopyError
		if !errors.As(err, &copyErr) || copyErr.Origin != oras.CopyErrorOriginSource || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("copyMultiplePlatforms() error = %v, want source resolve error", err)
		}
	})
	t.Run("malformed index", func(t *testing.T) {
		indexContent := []byte("{")
		root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
		src := &fetchOverrideTarget{ReadOnlyGraphTarget: memory.New(), root: root, data: indexContent}
		opts := &copyOptions{}
		opts.From.Reference = "source"
		if err := copyMultiplePlatforms(ctx, logrus.New(), nil, nil, src, memory.New(), opts); err == nil || !strings.Contains(err.Error(), "failed to parse index") {
			t.Fatalf("copyMultiplePlatforms() error = %v, want index parse error", err)
		}
	})
	t.Run("index fetch failure", func(t *testing.T) {
		indexContent := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
		root := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, indexContent)
		src := &fetchOverrideTarget{ReadOnlyGraphTarget: memory.New(), root: root, err: errors.New("fetch failed")}
		opts := &copyOptions{}
		opts.From.Reference = "source"
		if err := copyMultiplePlatforms(ctx, logrus.New(), nil, nil, src, memory.New(), opts); err == nil || !strings.Contains(err.Error(), "failed to fetch index") {
			t.Fatalf("copyMultiplePlatforms() error = %v, want fetch error", err)
		}
	})
	t.Run("unmatched requested platform", func(t *testing.T) {
		src, _, _ := newMultiPlatformSource(t)
		opts := &copyOptions{Platforms: option.Platforms{Platforms: []*ocispec.Platform{{OS: "windows", Architecture: "amd64"}}}}
		opts.From.Reference = "source"
		err := copyMultiplePlatforms(ctx, logrus.New(), nil, nil, src, memory.New(), opts)
		var platformError *oerrors.Error
		if !errors.As(err, &platformError) || !strings.Contains(err.Error(), "unmatched platforms") {
			t.Fatalf("copyMultiplePlatforms() error = %v, want unmatched platform error", err)
		}
	})
}

func Test_doMultipleCopy_rejectsInvalidFilteredIndex(t *testing.T) {
	opts := &copyOptions{}
	err := doMultipleCopy(context.Background(), logrus.New(), nil, nil, memory.New(), memory.New(), opts,
		ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex}, []byte(`[]`), nil)
	if err == nil || !strings.Contains(err.Error(), "failed to filter index") {
		t.Fatalf("doMultipleCopy() error = %v, want filtered-index error", err)
	}
}

func Test_copyCmd_rejectsInvalidPlatform(t *testing.T) {
	cmd := copyCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--platform", "linux/amd64/variant/extra", "source", "destination"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "failed to parse platform") {
		t.Fatalf("copyCmd().Execute() error = %v, want invalid platform error", err)
	}
}

func Test_copyCmd_platformSelection(t *testing.T) {
	for _, tt := range []struct {
		name              string
		platforms         string
		wantMediaType     string
		wantManifestCount int
	}{
		{name: "single platform keeps manifest output", platforms: "linux/amd64", wantMediaType: ocispec.MediaTypeImageManifest},
		{name: "multiple platforms produce filtered index", platforms: "linux/amd64,linux/arm/v7", wantMediaType: ocispec.MediaTypeImageIndex, wantManifestCount: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			source, _, _ := newMultiPlatformSource(t)
			sourcePath := filepath.Join(t.TempDir(), "source")
			sourceLayout, err := oci.New(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := oras.Copy(ctx, source, "source", sourceLayout, "source", oras.DefaultCopyOptions); err != nil {
				t.Fatalf("failed to prepare source layout: %v", err)
			}

			destinationPath := filepath.Join(t.TempDir(), "destination")
			cmd := copyCmd()
			cmd.SetContext(ctx)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{
				"--from-oci-layout", "--to-oci-layout", "--platform", tt.platforms,
				sourcePath + ":source", destinationPath + ":destination",
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("copy command failed: %v", err)
			}

			destination, err := oci.New(destinationPath)
			if err != nil {
				t.Fatal(err)
			}
			root, err := destination.Resolve(ctx, "destination")
			if err != nil {
				t.Fatal(err)
			}
			if root.MediaType != tt.wantMediaType {
				t.Fatalf("destination media type = %q, want %q", root.MediaType, tt.wantMediaType)
			}
			if tt.wantManifestCount > 0 {
				data, err := content.FetchAll(ctx, destination, root)
				if err != nil {
					t.Fatal(err)
				}
				var index ocispec.Index
				if err := json.Unmarshal(data, &index); err != nil {
					t.Fatal(err)
				}
				if len(index.Manifests) != tt.wantManifestCount || index.Manifests[1].Platform.Variant != "v7" {
					t.Fatalf("filtered manifests = %#v, want amd64 and arm/v7", index.Manifests)
				}
			}
		})
	}
}

func Test_copyCmd_invalidExtraTag(t *testing.T) {
	cmd := copyCmd()
	cmd.SetArgs([]string{
		"localhost:5000/source:v1",
		"localhost:5000/destination:v1,,invalid@@tag",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != "empty extra tag" {
		t.Fatalf("got %v, want empty extra tag error", err)
	}
}
