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

package json

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/cmd/oras/internal/option"
)

const testDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestNewPushHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf)
	if handler == nil {
		t.Fatal("NewPushHandler() returned nil")
	}
}

func TestPushHandler_OnTagged(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}

	if err := handler.OnTagged(desc, "v1.0.0"); err != nil {
		t.Fatalf("PushHandler.OnTagged() error = %v, want nil", err)
	}

	tags := handler.tagged.Tags()
	if !slices.Contains(tags, "v1.0.0") {
		t.Errorf("PushHandler.OnTagged() did not store tag: got tags = %v, want to contain %q", tags, "v1.0.0")
	}
}

func TestPushHandler_OnCopied(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}

	opts := &option.Target{
		RawReference: "localhost:5000/test:v1.0.0",
		Path:         "localhost:5000/test",
		Reference:    "v1.0.0",
	}

	if err := handler.OnCopied(opts, desc); err != nil {
		t.Fatalf("PushHandler.OnCopied() error = %v, want nil", err)
	}

	if handler.path != opts.Path {
		t.Errorf("PushHandler.OnCopied() path = %q, want %q", handler.path, opts.Path)
	}
	if handler.root.Digest != desc.Digest {
		t.Errorf("PushHandler.OnCopied() root.Digest = %q, want %q", handler.root.Digest, desc.Digest)
	}
	tags := handler.tagged.Tags()
	if !slices.Contains(tags, "v1.0.0") {
		t.Errorf("PushHandler.OnCopied() did not store tag for non-digest reference: got tags = %v", tags)
	}
}

func TestPushHandler_OnCopied_WithDigest(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}

	opts := &option.Target{
		RawReference: "localhost:5000/test@" + testDigest,
		Path:         "localhost:5000/test",
		Reference:    testDigest,
	}

	if err := handler.OnCopied(opts, desc); err != nil {
		t.Fatalf("PushHandler.OnCopied() error = %v, want nil", err)
	}

	if tags := handler.tagged.Tags(); len(tags) != 0 {
		t.Errorf("PushHandler.OnCopied() with digest reference should not add a tag, got tags = %v", tags)
	}
	if handler.path != opts.Path {
		t.Errorf("PushHandler.OnCopied() path = %q, want %q", handler.path, opts.Path)
	}
	if handler.root.Digest != desc.Digest {
		t.Errorf("PushHandler.OnCopied() root.Digest = %q, want %q", handler.root.Digest, desc.Digest)
	}
}

func TestPushHandler_Render(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}

	opts := &option.Target{
		RawReference: "localhost:5000/test:v1.0.0",
		Path:         "localhost:5000/test",
		Reference:    "v1.0.0",
	}

	if err := handler.OnCopied(opts, desc); err != nil {
		t.Fatalf("PushHandler.OnCopied() error = %v", err)
	}

	if err := handler.Render(); err != nil {
		t.Fatalf("PushHandler.Render() error = %v, want nil", err)
	}

	var result struct {
		Reference       string   `json:"reference"`
		MediaType       string   `json:"mediaType"`
		Digest          string   `json:"digest"`
		Size            int64    `json:"size"`
		ReferenceAsTags []string `json:"referenceAsTags"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("PushHandler.Render() produced invalid JSON: %v", err)
	}

	wantReference := opts.Path + "@" + testDigest
	if result.Reference != wantReference {
		t.Errorf("Render() reference = %q, want %q", result.Reference, wantReference)
	}
	if result.MediaType != desc.MediaType {
		t.Errorf("Render() mediaType = %q, want %q", result.MediaType, desc.MediaType)
	}
	if result.Digest != string(desc.Digest) {
		t.Errorf("Render() digest = %q, want %q", result.Digest, desc.Digest)
	}
	if result.Size != desc.Size {
		t.Errorf("Render() size = %d, want %d", result.Size, desc.Size)
	}
	wantRefAsTags := []string{"localhost:5000/test:v1.0.0"}
	if !slices.Equal(result.ReferenceAsTags, wantRefAsTags) {
		t.Errorf("Render() referenceAsTags = %v, want %v", result.ReferenceAsTags, wantRefAsTags)
	}
}

// An untagged push must report referenceAsTags as an empty array, not null,
// consistent with the repositories and tags lists of repo ls and repo tags.
func TestPushHandler_Render_untagged(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}
	if err := handler.OnCopied(&option.Target{Path: "localhost:5000/test"}, desc); err != nil {
		t.Fatalf("PushHandler.OnCopied() error = %v", err)
	}
	if err := handler.Render(); err != nil {
		t.Fatalf("PushHandler.Render() error = %v, want nil", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("PushHandler.Render() produced invalid JSON: %v", err)
	}
	if got := string(result["referenceAsTags"]); got != "[]" {
		t.Errorf("Render() referenceAsTags = %s, want []", got)
	}
}

// `oras push localhost:5000/test` parses to a target whose raw reference is
// the repository and whose reference is empty. Nothing was tagged, so no
// reference must be reported as a tag.
func TestPushHandler_Render_untaggedRawReference(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf).(*PushHandler)

	desc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    testDigest,
		Size:      100,
	}
	opts := &option.Target{
		RawReference: "localhost:5000/test",
		Path:         "localhost:5000/test",
	}
	if err := handler.OnCopied(opts, desc); err != nil {
		t.Fatalf("PushHandler.OnCopied() error = %v", err)
	}
	if err := handler.Render(); err != nil {
		t.Fatalf("PushHandler.Render() error = %v, want nil", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("PushHandler.Render() produced invalid JSON: %v", err)
	}
	if got := string(result["referenceAsTags"]); got != "[]" {
		t.Errorf("Render() referenceAsTags = %s, want []", got)
	}
}
