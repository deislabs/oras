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

package manifest

import (
	"bytes"
	"context"
	"encoding/json"
	stderrs "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/cmd/oras/internal/option"
	"oras.land/oras/cmd/oras/root/manifest/common"
)

type testConfigTarget struct {
	content []byte
}

func (t *testConfigTarget) Exists(context.Context, ocispec.Descriptor) (bool, error) {
	return false, nil
}

func (t *testConfigTarget) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(t.content)), nil
}

func (t *testConfigTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, nil
}

var _ oras.ReadOnlyTarget = (*testConfigTarget)(nil)

func manifestFetchTarget(t *testing.T) oras.ReadOnlyTarget {
	t.Helper()
	ctx := context.Background()
	target := memory.New()
	manifest := []byte(`{"schemaVersion":2}`)
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifest),
		Size:      int64(len(manifest)),
	}
	if err := target.Push(ctx, desc, bytes.NewReader(manifest)); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if err := target.Tag(ctx, desc, "test"); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}
	return target
}

func Test_fetchManifest_errType(t *testing.T) {
	// prepare
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// test
	opts := &fetchOptions{
		Format: option.Format{
			Type: "unknown",
		},
	}
	got := fetchManifest(cmd, opts).Error()
	want := errors.UnsupportedFormatTypeError(opts.Format.Type).Error()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test_doFetchManifest_contentPathHandlerCalls(t *testing.T) {
	recorder := &displaytest.CallRecorder{}
	contentHandler := &displaytest.RecordingManifestFetchContentHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingManifestFetchMetadataHandler{Recorder: recorder}
	opts := &fetchOptions{Target: option.Target{Reference: "test", RawReference: "test"}}

	if err := doFetchManifest(context.Background(), manifestFetchTarget(t), opts, metadataHandler, contentHandler); err != nil {
		t.Fatalf("doFetchManifest() error = %v", err)
	}
	if got, want := recorder.Snapshot(), []string{"OnContentFetched", "OnFetched"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doFetchManifest_contentHandlerError(t *testing.T) {
	wantedErr := stderrs.New("content handler failed")
	recorder := &displaytest.CallRecorder{}
	contentHandler := &displaytest.RecordingManifestFetchContentHandler{Recorder: recorder, ContentFetchedErr: wantedErr}
	metadataHandler := &displaytest.RecordingManifestFetchMetadataHandler{Recorder: recorder}
	opts := &fetchOptions{Target: option.Target{Reference: "test", RawReference: "test"}}

	err := doFetchManifest(context.Background(), manifestFetchTarget(t), opts, metadataHandler, contentHandler)
	if !stderrs.Is(err, wantedErr) {
		t.Fatalf("doFetchManifest() error = %v, want %v", err, wantedErr)
	}
	got := recorder.Snapshot()
	if len(got) != 1 || got[0] != "OnContentFetched" {
		t.Fatalf("handler calls = %v, want [OnContentFetched]", got)
	}
}

func Test_doFetchManifest_descriptorOnly(t *testing.T) {
	recorder := &displaytest.CallRecorder{}
	contentHandler := &displaytest.RecordingManifestFetchContentHandler{Recorder: recorder}
	metadataHandler := &displaytest.RecordingManifestFetchMetadataHandler{Recorder: recorder}
	opts := &fetchOptions{
		Target:     option.Target{Reference: "test", RawReference: "test"},
		Descriptor: option.Descriptor{OutputDescriptor: true},
	}

	if err := doFetchManifest(context.Background(), manifestFetchTarget(t), opts, metadataHandler, contentHandler); err != nil {
		t.Fatalf("doFetchManifest() error = %v", err)
	}
	if got, want := recorder.Snapshot(), []string{"OnFetched"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("metadata handler calls = %v, want %v", got, want)
	}
}

func Test_fetchCmd_outputAndFormat(t *testing.T) {
	cmd := fetchCmd()
	cmd.SetArgs([]string{
		"localhost:5000/hello:v1",
		"--output", "-",
		"--format", "go-template",
		"--template", "{{.digest}}",
	})

	err := cmd.Execute()
	want := "`--output -` cannot be used with `--format go-template` at the same time"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestFetchManifest_ociLayoutExecution(t *testing.T) {
	ctx := context.Background()
	layout := t.TempDir()
	store, err := oci.New(layout)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Tag(ctx, desc, "source"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "manifest.json")
	rootCmd := &cobra.Command{Use: "oras"}
	rootCmd.AddCommand(fetchCmd())
	rootCmd.SetArgs([]string{"fetch", "--oci-layout-path", layout, "source", "--output", out})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Fatalf("manifest output invalid: info=%v err=%v", info, err)
	}
}

func Test_fetchConfigContent_sizeExceedsLimit(t *testing.T) {
	target := &testConfigTarget{}
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Size:      common.MaxConfigSize + 1,
	}

	_, err := fetchConfigContent(context.Background(), target, desc)
	if !stderrs.Is(err, errdef.ErrSizeExceedsLimit) {
		t.Fatalf("got %v, want %v", err, errdef.ErrSizeExceedsLimit)
	}
}

func Test_fetchConfig_outputStdout_exceedsLimit(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")

	config := bytes.Repeat([]byte("a"), int(common.MaxConfigSize)+1)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(
		filepath.Join(layoutDir, "index.json"),
		index,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	cmd.SetOut(&output)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", "-",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(output.Bytes(), config) {
		t.Fatalf("got %d bytes, want %d", output.Len(), len(config))
	}
}

func Test_fetchConfig_outputStdoutPretty_exceedsLimit(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")

	config := bytes.Repeat([]byte("a"), int(common.MaxConfigSize)+1)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"--output", "-",
		"--pretty",
		"test:v1",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected config size limit error")
	}
	if !strings.Contains(err.Error(), "exceeds the limit") {
		t.Fatalf("got %q, want config size limit error", err)
	}
}

func Test_fetchConfig_outputStdoutPretty(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")

	config := []byte(`{"architecture":"amd64"}`)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	cmd.SetOut(&output)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"--output", "-",
		"--pretty",
		"test:v1",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"architecture\": \"amd64\"\n}\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func Test_fetchConfigContent_withinLimit(t *testing.T) {
	data := []byte(`{"architecture":"amd64"}`)
	target := &testConfigTarget{
		content: data,
	}
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}

	got, err := fetchConfigContent(context.Background(), target, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func Test_fetchConfigDesc_rejectsNonImageManifest(t *testing.T) {
	ctx := context.Background()
	target := memory.New()
	content := []byte(`{"config": {}}`)
	desc := ocispec.Descriptor{
		MediaType: "application/json",
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
	}
	if err := target.Push(ctx, desc, bytes.NewReader(content)); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if err := target.Tag(ctx, desc, "test"); err != nil {
		t.Fatalf("Tag() error = %v", err)
	}

	_, err := fetchConfigDesc(ctx, target, "test", nil)
	if err == nil || !strings.Contains(err.Error(), "is not an image manifest and does not have a config") {
		t.Fatalf("fetchConfigDesc() error = %v, want non-image-manifest error", err)
	}
}

func Test_fetchConfig_outputFile(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")
	outputPath := filepath.Join(tempDir, "config.json")

	config := bytes.Repeat([]byte("a"), int(common.MaxConfigSize)+1)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	ociLayout := []byte(`{"imageLayoutVersion":"1.0.0"}`)
	if err := os.WriteFile(filepath.Join(layoutDir, "oci-layout"), ociLayout, 0644); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))
	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	cmd.SetOut(&output)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", outputPath,
		"--descriptor",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, config) {
		t.Fatalf("got %q, want %q", got, config)
	}
	var gotDesc ocispec.Descriptor
	if err := json.Unmarshal(output.Bytes(), &gotDesc); err != nil {
		t.Fatalf("failed to decode descriptor output: %v", err)
	}
	if gotDesc.Digest != configDigest {
		t.Fatalf("got digest %q, want %q", gotDesc.Digest, configDigest)
	}
}

func Test_fetchConfig_outputStdout(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")

	config := []byte(`{"architecture":"amd64"}`)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "config": {
            "mediaType": %q,
            "digest": %q,
            "size": %d
        },
        "layers": []
    }`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "manifests": [{
            "mediaType": %q,
            "digest": %q,
            "size": %d,
            "annotations": {
                "org.opencontainers.image.ref.name": "test:v1"
            }
        }]
    }`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	cmd.SetOut(&output)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), config) {
		t.Fatalf("got %q, want %q", output.Bytes(), config)
	}
}

func Test_fetchConfig_outputFile_fetchError(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")
	outputPath := filepath.Join(tempDir, "config.json")

	configDigest := digest.FromString("missing-config")

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": 100
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(
		filepath.Join(layoutDir, "index.json"),
		index,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", outputPath,
	})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected fetch error")
	}

	if _, err := os.Stat(outputPath); err == nil {
		t.Fatal("output file was created after fetch failed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed to check output file: %v", err)
	}
}

func Test_fetchConfig_outputFile_createError(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")
	outputPath := filepath.Join(tempDir, "missing", "config.json")

	config := []byte(`{"architecture":"amd64"}`)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", outputPath,
	})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected output file creation error")
	}
}

func Test_fetchConfig_outputFile_renameError(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")
	outputPath := filepath.Join(tempDir, "config.json")

	config := []byte(`{"architecture":"amd64"}`)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "config": {
            "mediaType": %q,
            "digest": %q,
            "size": %d
        },
        "layers": []
    }`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "manifests": [{
            "mediaType": %q,
            "digest": %q,
            "size": %d,
            "annotations": {
                "org.opencontainers.image.ref.name": "test:v1"
            }
        }]
    }`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(
		filepath.Join(layoutDir, "index.json"),
		index,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(outputPath, 0755); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", outputPath,
	})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected rename error")
	}
}

func Test_fetchConfig_descriptorOnly(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")

	config := []byte(`{"architecture":"amd64"}`)
	configDigest := digest.FromBytes(config)

	manifest := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "config": {
            "mediaType": %q,
            "digest": %q,
            "size": %d
        },
        "layers": []
    }`, ocispec.MediaTypeImageConfig, configDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "oci-layout"),
		[]byte(`{"imageLayoutVersion":"1.0.0"}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
        "schemaVersion": 2,
        "manifests": [{
            "mediaType": %q,
            "digest": %q,
            "size": %d,
            "annotations": {
                "org.opencontainers.image.ref.name": "test:v1"
            }
        }]
    }`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))

	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", configDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	cmd.SetOut(&output)
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--descriptor",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var got ocispec.Descriptor
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Digest != configDigest {
		t.Fatalf("got digest %q, want %q", got.Digest, configDigest)
	}
}

func Test_fetchConfig_outputFile_digestMismatch(t *testing.T) {
	tempDir := t.TempDir()
	layoutDir := filepath.Join(tempDir, "layout")
	outputPath := filepath.Join(tempDir, "config.json")

	original := []byte("original config")
	if err := os.WriteFile(outputPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	config := []byte(`{"architecture":"amd64"}`)
	expectedDigest := digest.FromString("expected")

	manifest := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"config": {
			"mediaType": %q,
			"digest": %q,
			"size": %d
		},
		"layers": []
	}`, ocispec.MediaTypeImageConfig, expectedDigest, len(config)))
	manifestDigest := digest.FromBytes(manifest)

	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", "sha256"), 0755); err != nil {
		t.Fatal(err)
	}

	ociLayout := []byte(`{"imageLayoutVersion":"1.0.0"}`)
	if err := os.WriteFile(filepath.Join(layoutDir, "oci-layout"), ociLayout, 0644); err != nil {
		t.Fatal(err)
	}

	index := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"manifests": [{
			"mediaType": %q,
			"digest": %q,
			"size": %d,
			"annotations": {
				"org.opencontainers.image.ref.name": "test:v1"
			}
		}]
	}`, ocispec.MediaTypeImageManifest, manifestDigest, len(manifest)))
	if err := os.WriteFile(filepath.Join(layoutDir, "index.json"), index, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", expectedDigest.Encoded()),
		config,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		filepath.Join(layoutDir, "blobs", "sha256", manifestDigest.Encoded()),
		manifest,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "oras"}
	cmd := fetchConfigCmd()
	rootCmd.AddCommand(cmd)
	rootCmd.SetArgs([]string{
		"fetch-config",
		"--oci-layout-path", layoutDir,
		"test:v1",
		"--output", outputPath,
	})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected digest mismatch error")
	} else if !strings.Contains(err.Error(), "mismatched digest") {
		t.Fatalf("got %v, want mismatched digest error", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("output file was modified: got %q, want %q", got, original)
	}
}
