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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/content/oci"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/sirupsen/logrus"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
)

func TestParseArtifactReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantRepo string
		wantTags []string
		wantErr  bool
	}{
		// Valid cases
		{
			name:     "valid reference with single tag",
			input:    "localhost:5000/repo:v1",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{"v1"},
			wantErr:  false,
		},
		{
			name:     "valid reference with multiple tags",
			input:    "localhost:5000/repo:v1,v2,v3",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{"v1", "v2", "v3"},
			wantErr:  false,
		},
		{
			name:     "complex repository path",
			input:    "localhost:5000/org/team/project:v1,v2",
			wantRepo: "localhost:5000/org/team/project",
			wantTags: []string{"v1", "v2"},
			wantErr:  false,
		},
		{
			name:     "reference without tag",
			input:    "localhost:5000/repo",
			wantRepo: "localhost:5000/repo",
			wantTags: nil,
			wantErr:  false,
		},
		{
			name:     "reference with empty tag",
			input:    "localhost:5000/repo:",
			wantRepo: "localhost:5000/repo",
			wantTags: nil,
			wantErr:  false,
		},
		{
			name:     "valid tag with allowed characters",
			input:    "localhost:5000/repo:v1.0-beta_1",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{"v1.0-beta_1"},
			wantErr:  false,
		},
		// Invalid cases
		{
			name:     "empty reference",
			input:    "",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "empty tag in middle of list",
			input:    "localhost:5000/repo:v1,,v2",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{},
			wantErr:  true,
		},
		{
			name:     "empty first tag with valid second tag",
			input:    "localhost:5000/repo:,v1",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{},
			wantErr:  true,
		},
		{
			name:     "multiple empty tags",
			input:    "localhost:5000/repo:,,",
			wantRepo: "localhost:5000/repo",
			wantTags: []string{},
			wantErr:  true,
		},
		{
			name:     "digest reference not supported",
			input:    "localhost:5000/repo@sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "digest with additional tags",
			input:    "localhost:5000/repo@sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447,v1",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "invalid tag format with special chars",
			input:    "localhost:5000/repo:valid,invalid!tag",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "no repository and tag specified",
			input:    "localhost:5000",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "no repository specified",
			input:    "localhost:5000:v1",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "invalid repository format with space",
			input:    "localhost:5000/repo space:v1",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "invalid tag starting with non-word character",
			input:    "localhost:5000/repo:.invalid",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
		{
			name:     "malformed reference with multiple colons",
			input:    "localhost:5000:abc/repo:v1",
			wantRepo: "",
			wantTags: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, references, err := parseArtifactReferences(tt.input)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Validate results
			if repository != tt.wantRepo {
				t.Errorf("expected repository %q, got %q", tt.wantRepo, repository)
			}
			if !reflect.DeepEqual(references, tt.wantTags) {
				t.Errorf("expected references %v, got %v", tt.wantTags, references)
			}
		})
	}
}

func newBackupTestSource(t *testing.T) oras.GraphTarget {
	t.Helper()
	src := memory.New()
	desc, err := oras.PackManifest(context.Background(), src, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Tag(context.Background(), desc, "v1"); err != nil {
		t.Fatal(err)
	}
	return src
}

func Test_doBackup_handlerCalls(t *testing.T) {
	ctx := context.Background()
	dstRoot := t.TempDir()
	dst, err := oci.New(dstRoot)
	if err != nil {
		t.Fatal(err)
	}
	opts := &backupOptions{output: dstRoot, outputFormat: outputFormatDir, repository: "example", tags: []string{"v1"}}
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingBackupStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingBackupMetadataHandler{Recorder: recorder}

	if err := doBackup(ctx, opts, logrus.New(), newBackupTestSource(t), dst, dstRoot, time.Now(), statusHandler, metadataHandler); err != nil {
		t.Fatal(err)
	}

	calls := recorder.Snapshot()
	if len(calls) < 5 {
		t.Fatalf("handler calls = %v, want at least 5", calls)
	}
	if got, want := calls[:2], []string{"OnTagsFound", "StartTracking"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first handler calls = %v, want %v", got, want)
	}
	if got, want := calls[len(calls)-3:], []string{"StopTracking", "OnArtifactPulled", "OnBackupCompleted"}; !reflect.DeepEqual(got, want) {
		t.Errorf("last handler calls = %v, want %v", got, want)
	}
}

func Test_doBackup_tagsFoundError(t *testing.T) {
	ctx := context.Background()
	dstRoot := t.TempDir()
	dst, err := oci.New(dstRoot)
	if err != nil {
		t.Fatal(err)
	}
	opts := &backupOptions{output: dstRoot, outputFormat: outputFormatDir, repository: "example", tags: []string{"v1"}}
	wantErr := errors.New("tags found failed")
	recorder := &displaytest.CallRecorder{}
	statusHandler := &displaytest.RecordingBackupStatusHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingBackupMetadataHandler{Recorder: recorder, TagsFoundErr: wantErr}

	err = doBackup(ctx, opts, logrus.New(), newBackupTestSource(t), dst, dstRoot, time.Now(), statusHandler, metadataHandler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("doBackup() error = %v, want %v", err, wantErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnTagsFound"}; !reflect.DeepEqual(got, want) {
		t.Errorf("handler calls = %v, want %v", got, want)
	}
}

func TestFinalizeBackupOutput(t *testing.T) {
	mockLogger := &mockLogger{}

	t.Run("Directory output format", func(t *testing.T) {
		tempDir := t.TempDir()
		// Create an ingest directory to ensure it gets removed
		dstRoot := filepath.Join(tempDir, "root")
		ingestDir := filepath.Join(dstRoot, "ingest")
		if err := os.MkdirAll(ingestDir, 0755); err != nil {
			t.Fatalf("Failed to create ingest dir: %v", err)
		}

		mockHandler := &mockBackupHandler{}
		opts := &backupOptions{
			outputFormat: outputFormatDir,
			output:       filepath.Join(tempDir, "output-dir"),
		}

		err := finalizeBackupOutput(dstRoot, opts, mockLogger, mockHandler)
		if err != nil {
			t.Errorf("Expected no error for directory output, got: %v", err)
		}

		// Ensure ingest directory was removed
		if _, err := os.Stat(ingestDir); !os.IsNotExist(err) {
			t.Errorf("Expected ingest directory to be removed")
		}

		// Verify handler methods weren't called for directory output
		if mockHandler.tarExportingCalled {
			t.Errorf("OnTarExporting should not be called for directory output")
		}
		if mockHandler.tarExportedCalled {
			t.Errorf("OnTarExported should not be called for directory output")
		}
	})

	t.Run("Tar output format", func(t *testing.T) {
		tempDir := t.TempDir()
		// Create an ingest directory to ensure it gets removed
		dstRoot := filepath.Join(tempDir, "root")
		ingestDir := filepath.Join(dstRoot, "ingest")
		if err := os.MkdirAll(ingestDir, 0755); err != nil {
			t.Fatalf("Failed to create ingest dir: %v", err)
		}

		outputPath := filepath.Join(tempDir, "output.tar")
		opts := &backupOptions{
			outputFormat: outputFormatTar,
			output:       outputPath,
		}

		mockHandler := &mockBackupHandler{}
		err := finalizeBackupOutput(dstRoot, opts, mockLogger, mockHandler)
		if err != nil {
			t.Fatalf("Expected no error for tar output, got: %v", err)
		}

		// Check if tar file was created
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Errorf("Expected tar file to exist at %s", outputPath)
		}

		// Verify handler methods were called for tar output
		if !mockHandler.tarExportingCalled {
			t.Errorf("OnTarExporting wasn't called")
		}
		if !mockHandler.tarExportedCalled {
			t.Errorf("OnTarExported wasn't called")
		}

		// Clean up
		if err := os.Remove(outputPath); err != nil {
			t.Logf("Failed to remove output tar file: %v", err)
		}
	})

	t.Run("Error in OnTarExporting", func(t *testing.T) {
		tempDir := t.TempDir()
		dstRoot := filepath.Join(tempDir, "root")
		ingestDir := filepath.Join(dstRoot, "ingest")
		if err := os.MkdirAll(ingestDir, 0755); err != nil {
			t.Fatalf("Failed to create ingest dir: %v", err)
		}

		opts := &backupOptions{
			outputFormat: outputFormatTar,
			output:       filepath.Join(tempDir, "error.tar"),
		}

		expectedErr := fmt.Errorf("export error")
		mockHandler := &mockBackupHandler{
			tarExportingResult: expectedErr,
		}

		err := finalizeBackupOutput(dstRoot, opts, mockLogger, mockHandler)
		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got: %v", expectedErr, err)
		}
	})

	t.Run("Error in OnTarExported", func(t *testing.T) {
		tempDir := t.TempDir()
		dstRoot := filepath.Join(tempDir, "root")
		ingestDir := filepath.Join(dstRoot, "ingest")
		if err := os.MkdirAll(ingestDir, 0755); err != nil {
			t.Fatalf("Failed to create ingest dir: %v", err)
		}

		opts := &backupOptions{
			outputFormat: outputFormatTar,
			output:       filepath.Join(tempDir, "error.tar"),
		}

		expectedErr := fmt.Errorf("tar exported error")
		mockHandler := &mockBackupHandler{
			tarExportedResult: expectedErr,
		}

		err := finalizeBackupOutput(dstRoot, opts, mockLogger, mockHandler)
		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got: %v", expectedErr, err)
		}
	})

	t.Run("Non-existent output directory", func(t *testing.T) {
		tempDir := t.TempDir()
		// Create a temporary directory that we can control
		nonExistentDir := filepath.Join(tempDir, "non-existent")

		// Make sure it doesn't exist by removing it if it does
		_ = os.RemoveAll(nonExistentDir)

		// Setup a path in a non-existent directory that should trigger mkdir
		outputPath := filepath.Join(nonExistentDir, "nested", "output.tar")

		opts := &backupOptions{
			outputFormat: outputFormatTar,
			output:       outputPath,
		}

		mockHandler := &mockBackupHandler{}
		if err := finalizeBackupOutput(tempDir, opts, mockLogger, mockHandler); err == nil {
			t.Errorf("Expected error for non-existent output directory, got nil")
		}
	})
}

func Test_resolveTags(t *testing.T) {
	ctx := context.Background()
	repoName := "test/repo"

	// Mock descriptors
	desc1 := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    "sha256:d5b7c742df27379894518554b73f7a3a03b4440ea435151a8b525a8d2555a0b2",
		Size:      123,
	}
	desc2 := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    "sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
		Size:      456,
	}

	// test server setup
	setupServer := func(handlers map[string]http.HandlerFunc) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("/v2/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
			w.WriteHeader(http.StatusOK)
		})
		for path, handler := range handlers {
			mux.HandleFunc(path, handler)
		}
		return httptest.NewServer(mux)
	}

	// Common manifest handler
	manifestHandler := func(desc ocispec.Descriptor) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", desc.MediaType)
			w.Header().Set("Docker-Content-Digest", desc.Digest.String())
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, desc.Size))
		}
	}

	t.Run("with specified tags", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/manifests/v1", repoName): manifestHandler(desc1),
			fmt.Sprintf("/v2/%s/manifests/v2", repoName): manifestHandler(desc2),
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		tags, descs, err := resolveTags(ctx, repo, []string{"v1", "v2"})
		if err != nil {
			t.Fatalf("resolveTags() error = %v, wantErr nil", err)
		}
		if !reflect.DeepEqual(tags, []string{"v1", "v2"}) {
			t.Errorf("resolveTags() tags = %v, want %v", tags, []string{"v1", "v2"})
		}
		if len(descs) != 2 {
			t.Fatalf("resolveTags() expected 2 descriptors, got %d", len(descs))
		}
		if descs[0].Digest != desc1.Digest {
			t.Errorf("resolveTags() desc[0] digest = %v, want %v", descs[0].Digest, desc1.Digest)
		}
		if descs[1].Digest != desc2.Digest {
			t.Errorf("resolveTags() desc[1] digest = %v, want %v", descs[1].Digest, desc2.Digest)
		}
	})

	t.Run("error resolving specified tag", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/manifests/non-existent", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		_, _, err = resolveTags(ctx, repo, []string{"non-existent"})
		if wantErr := errdef.ErrNotFound; !errors.Is(err, wantErr) {
			t.Errorf("resolveTags() error = %v, wantErr %v", err, wantErr)
		}
	})

	t.Run("fetching all tags from repository", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/tags/list", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"` + repoName + `","tags":["v1","v2"]}`))
			},
			fmt.Sprintf("/v2/%s/manifests/v1", repoName): manifestHandler(desc1),
			fmt.Sprintf("/v2/%s/manifests/v2", repoName): manifestHandler(desc2),
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		tags, descs, err := resolveTags(ctx, repo, nil)
		if err != nil {
			t.Fatalf("resolveTags() error = %v, wantErr nil", err)
		}
		expectedTags := []string{"v1", "v2"}
		if !reflect.DeepEqual(tags, expectedTags) {
			t.Errorf("resolveTags() tags = %v, want %v", tags, expectedTags)
		}
		if len(descs) != 2 {
			t.Fatalf("resolveTags() expected 2 descriptors, got %d", len(descs))
		}
		if descs[0].Digest != desc1.Digest {
			t.Errorf("resolveTags() desc[0] digest = %v, want %v", descs[0].Digest, desc1.Digest)
		}
		if descs[1].Digest != desc2.Digest {
			t.Errorf("resolveTags() desc[1] digest = %v, want %v", descs[1].Digest, desc2.Digest)
		}
	})

	t.Run("error listing tags from repository", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/tags/list", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		_, _, err = resolveTags(ctx, repo, nil)
		if err == nil {
			t.Error("resolveTags() error = nil, wantErr not nil")
		}
	})

	t.Run("error resolving one of the listed tags", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/tags/list", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"` + repoName + `","tags":["v1","v2-bad"]}`))
			},
			fmt.Sprintf("/v2/%s/manifests/v1", repoName): manifestHandler(desc1),
			fmt.Sprintf("/v2/%s/manifests/v2-bad", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		_, _, err = resolveTags(ctx, repo, nil)
		if wantErr := errdef.ErrNotFound; !errors.Is(err, wantErr) {
			t.Errorf("resolveTags() error = %v, wantErr %v", err, wantErr)
		}
	})

	t.Run("empty tag list from repository", func(t *testing.T) {
		server := setupServer(map[string]http.HandlerFunc{
			fmt.Sprintf("/v2/%s/tags/list", repoName): func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"` + repoName + `","tags":[]}`))
			},
		})
		defer server.Close()

		repo, err := remote.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/" + repoName)
		if err != nil {
			t.Fatalf("failed to create remote repository: %v", err)
		}
		repo.Registry.PlainHTTP = true

		tags, descs, err := resolveTags(ctx, repo, nil)
		if err != nil {
			t.Fatalf("resolveTags() error = %v, wantErr nil", err)
		}
		if len(tags) != 0 {
			t.Errorf("resolveTags() tags = %v, want empty slice", tags)
		}
		if len(descs) != 0 {
			t.Errorf("resolveTags() descs = %v, want empty slice", descs)
		}
	})

	t.Run("target does not support tag listing", func(t *testing.T) {
		// Use a simple mock that doesn't implement registry.TagLister
		target := memory.New()
		_, _, err := resolveTags(ctx, target, nil)
		if wantErr := errTagListNotSupported; !errors.Is(err, wantErr) {
			t.Errorf("resolveTags() error = %v, wantErr %v", err, wantErr)
		}
	})
}

func Test_countReferrers(t *testing.T) {
	// prepare test data
	ctx := context.Background()
	target := memory.New()
	tag := "test"

	// create manifests
	manifestDesc1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/manifest1", oras.PackManifestOptions{})
	if err != nil {
		t.Fatalf("failed to create manifest descriptor 1: %v", err)
	}
	manifestDesc2, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/manifest2", oras.PackManifestOptions{})
	if err != nil {
		t.Fatalf("failed to create manifest descriptor 2: %v", err)
	}
	// create flatten referrers
	_, err = oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer1_1", oras.PackManifestOptions{Subject: &manifestDesc1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 1: %v", err)
	}
	_, err = oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer1_2", oras.PackManifestOptions{Subject: &manifestDesc1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 1: %v", err)
	}
	// create nested referrers
	referrerDesc2_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer2_1", oras.PackManifestOptions{Subject: &manifestDesc2})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 2: %v", err)
	}
	_, err = oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/referrer2_1_1", oras.PackManifestOptions{Subject: &referrerDesc2_1})
	if err != nil {
		t.Fatalf("failed to create referrer descriptor 2: %v", err)
	}
	// create index manifest
	index := ocispec.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Manifests: []ocispec.Descriptor{
			manifestDesc1,
			manifestDesc2,
		},
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("failed to marshal index manifest: %v", err)
	}
	indexDesc, err := oras.PushBytes(ctx, target, ocispec.MediaTypeImageIndex, indexBytes)
	if err != nil {
		t.Fatalf("failed to push index manifest: %v", err)
	}
	// add nested referrers to the index
	referrerDesc3_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/index", oras.PackManifestOptions{Subject: &indexDesc})
	if err != nil {
		t.Fatalf("failed to create index referrer: %v", err)
	}
	referrerDesc3_1_1, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "test/index_referrer1", oras.PackManifestOptions{Subject: &referrerDesc3_1})
	if err != nil {
		t.Fatalf("failed to create nested index referrer: %v", err)
	}

	t.Run("count flatten referrers for manifest 1", func(t *testing.T) {
		count, err := countReferrers(ctx, target, tag, manifestDesc1, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("countReferrers() error = %v, wantErr nil", err)
		}
		if wantCount := 2; count != wantCount {
			t.Errorf("countReferrers() count = %d, want %d", count, wantCount)
		}
	})

	t.Run("count nested referrers for manifest 2", func(t *testing.T) {
		count, err := countReferrers(ctx, target, tag, manifestDesc2, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("countReferrers() error = %v, wantErr nil", err)
		}
		if wantCount := 2; count != wantCount {
			t.Errorf("countReferrers() count = %d, want %d", count, wantCount)
		}
	})

	t.Run("count referrers for index", func(t *testing.T) {
		count, err := countReferrers(ctx, target, tag, indexDesc, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("countReferrers() error = %v, wantErr nil", err)
		}
		if wantCount := 6; count != wantCount {
			t.Errorf("countReferrers() count = %d, want %d", count, wantCount)
		}
	})

	t.Run("count referrers for manifest with no referrers", func(t *testing.T) {
		count, err := countReferrers(ctx, target, tag, referrerDesc3_1_1, oras.DefaultExtendedCopyGraphOptions)
		if err != nil {
			t.Fatalf("countReferrers() error = %v, wantErr nil", err)
		}
		if wantCount := 0; count != wantCount {
			t.Errorf("countReferrers() count = %d, want %d", count, wantCount)
		}
	})

	t.Run("bad FindPredecessors", func(t *testing.T) {
		testErr := errors.New("test error")
		opts := oras.ExtendedCopyGraphOptions{
			FindPredecessors: func(_ context.Context, _ content.ReadOnlyGraphStorage, _ ocispec.Descriptor) ([]ocispec.Descriptor, error) {
				return nil, testErr
			},
		}
		_, err := countReferrers(ctx, target, tag, manifestDesc1, opts)
		if !errors.Is(err, testErr) {
			t.Errorf("countReferrers() error = %v, want %v", err, testErr)
		}
	})
}

// Mock implementations
type mockLogger struct {
	debugMessages []string
}

func (m *mockLogger) WithField(key string, value any) *logrus.Entry {
	return logrus.WithField(key, value)
}

func (m *mockLogger) WithFields(fields logrus.Fields) *logrus.Entry {
	return logrus.WithFields(fields)
}

func (m *mockLogger) WithError(err error) *logrus.Entry {
	return logrus.WithError(err)
}

func (m *mockLogger) Debugf(format string, args ...any) {
	m.debugMessages = append(m.debugMessages, fmt.Sprintf(format, args...))
}

func (m *mockLogger) Infof(_ string, _ ...any)    {}
func (m *mockLogger) Printf(_ string, _ ...any)   {}
func (m *mockLogger) Warnf(_ string, _ ...any)    {}
func (m *mockLogger) Warningf(_ string, _ ...any) {}
func (m *mockLogger) Errorf(_ string, _ ...any)   {}
func (m *mockLogger) Fatalf(_ string, _ ...any)   {}
func (m *mockLogger) Panicf(_ string, _ ...any)   {}

func (m *mockLogger) Debug(_ ...any)   {}
func (m *mockLogger) Info(_ ...any)    {}
func (m *mockLogger) Print(_ ...any)   {}
func (m *mockLogger) Warn(_ ...any)    {}
func (m *mockLogger) Warning(_ ...any) {}
func (m *mockLogger) Error(_ ...any)   {}
func (m *mockLogger) Fatal(_ ...any)   {}
func (m *mockLogger) Panic(_ ...any)   {}

func (m *mockLogger) Debugln(_ ...any)   {}
func (m *mockLogger) Infoln(_ ...any)    {}
func (m *mockLogger) Println(_ ...any)   {}
func (m *mockLogger) Warnln(_ ...any)    {}
func (m *mockLogger) Warningln(_ ...any) {}
func (m *mockLogger) Errorln(_ ...any)   {}
func (m *mockLogger) Fatalln(_ ...any)   {}
func (m *mockLogger) Panicln(_ ...any)   {}

type mockBackupHandler struct {
	calls              []string
	tarExportingCalled bool
	tarExportedCalled  bool
	tarExportingResult error
	tarExportedResult  error
	tagsFoundResult    error
}

func (m *mockBackupHandler) StartTracking(gt oras.GraphTarget) (oras.GraphTarget, error) {
	m.calls = append(m.calls, "StartTracking")
	return gt, nil
}

func (m *mockBackupHandler) StopTracking() error {
	m.calls = append(m.calls, "StopTracking")
	return nil
}

func (m *mockBackupHandler) OnCopySkipped(context.Context, ocispec.Descriptor) error {
	m.calls = append(m.calls, "OnCopySkipped")
	return nil
}

func (m *mockBackupHandler) PreCopy(context.Context, ocispec.Descriptor) error {
	m.calls = append(m.calls, "PreCopy")
	return nil
}

func (m *mockBackupHandler) PostCopy(context.Context, ocispec.Descriptor) error {
	m.calls = append(m.calls, "PostCopy")
	return nil
}

func (m *mockBackupHandler) OnTarExporting(_ string) error {
	m.calls = append(m.calls, "OnTarExporting")
	m.tarExportingCalled = true
	return m.tarExportingResult
}

func (m *mockBackupHandler) OnTarExported(_ string, _ int64) error {
	m.calls = append(m.calls, "OnTarExported")
	m.tarExportedCalled = true
	return m.tarExportedResult
}

func (m *mockBackupHandler) OnTagsFound(_ []string) error {
	m.calls = append(m.calls, "OnTagsFound")
	return m.tagsFoundResult
}

func (m *mockBackupHandler) OnArtifactPulled(_ string, _ int) error {
	m.calls = append(m.calls, "OnArtifactPulled")
	return nil
}

func (m *mockBackupHandler) OnBackupCompleted(_ int, _ string, _ time.Duration) error {
	m.calls = append(m.calls, "OnBackupCompleted")
	return nil
}

func (m *mockBackupHandler) Render() error {
	return nil
}
