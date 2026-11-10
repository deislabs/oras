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

package template

import (
	"bytes"
	"slices"
	"testing"

	"oras.land/oras/cmd/oras/internal/option"
)

func TestNewPushHandler(t *testing.T) {
	if got := NewPushHandler(&bytes.Buffer{}, "{{.reference}}"); got == nil {
		t.Fatal("NewPushHandler() returned nil")
	}
}

func TestPushHandler_Render(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf, "{{.reference}}|{{range .referenceAsTags}}{{.}} {{end}}").(*PushHandler)

	if err := handler.OnTagged(testDescriptor, "v1.0.0"); err != nil {
		t.Fatalf("OnTagged() error = %v, want nil", err)
	}
	opts := &option.Target{
		RawReference: "localhost:5000/test:latest",
		Path:         "localhost:5000/test",
		Reference:    "latest",
	}
	if err := handler.OnCopied(opts, testDescriptor); err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}
	if err := handler.Render(); err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}

	// Tags are sorted, so "latest" precedes "v1.0.0" regardless of the order
	// they were recorded in.
	want := "localhost:5000/test@" + testDigest + "|localhost:5000/test:latest localhost:5000/test:v1.0.0 "
	if got := buf.String(); got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

// A digest reference is not a tag, so OnCopied must not record it as one.
func TestPushHandler_OnCopied_digestReferenceIsNotTagged(t *testing.T) {
	handler := NewPushHandler(&bytes.Buffer{}, "{{.reference}}").(*PushHandler)

	opts := &option.Target{
		RawReference: "localhost:5000/test@" + testDigest,
		Path:         "localhost:5000/test",
		Reference:    testDigest,
	}
	if err := handler.OnCopied(opts, testDescriptor); err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}
	if tags := handler.tagged.Tags(); len(tags) != 0 {
		t.Errorf("OnCopied() recorded tags = %v, want none for a digest reference", tags)
	}
}

// An empty raw reference means nothing was tagged.
func TestPushHandler_OnCopied_noReference(t *testing.T) {
	handler := NewPushHandler(&bytes.Buffer{}, "{{.reference}}").(*PushHandler)

	opts := &option.Target{Path: "localhost:5000/test"}
	if err := handler.OnCopied(opts, testDescriptor); err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}
	if tags := handler.tagged.Tags(); len(tags) != 0 {
		t.Errorf("OnCopied() recorded tags = %v, want none when no reference is given", tags)
	}
	if handler.path != "localhost:5000/test" {
		t.Errorf("OnCopied() path = %q, want %q", handler.path, "localhost:5000/test")
	}
}

func TestPushHandler_OnTagged(t *testing.T) {
	handler := NewPushHandler(&bytes.Buffer{}, "{{.reference}}").(*PushHandler)

	for _, tag := range []string{"v1.0.0", "latest"} {
		if err := handler.OnTagged(testDescriptor, tag); err != nil {
			t.Fatalf("OnTagged(%q) error = %v, want nil", tag, err)
		}
	}
	tags := handler.tagged.Tags()
	for _, want := range []string{"v1.0.0", "latest"} {
		if !slices.Contains(tags, want) {
			t.Errorf("OnTagged() tags = %v, want to contain %q", tags, want)
		}
	}
}

func TestPushHandler_Render_invalidTemplate(t *testing.T) {
	handler := NewPushHandler(&bytes.Buffer{}, "{{.reference")
	if err := handler.Render(); err == nil {
		t.Error("Render() error = nil, want an error for an unparsable template")
	}
}

// After an untagged push, a template must be able to take the length of
// referenceAsTags, as it can for the tags of repo tags and the repositories of
// repo ls.
func TestPushHandler_Render_untaggedLen(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf, "{{len .referenceAsTags}}").(*PushHandler)

	if err := handler.OnCopied(&option.Target{Path: "localhost:5000/test"}, testDescriptor); err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}
	if err := handler.Render(); err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}
	if got, want := buf.String(), "0"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

// `oras push localhost:5000/test` parses to a target whose raw reference is
// the repository and whose reference is empty. Nothing was tagged, so no
// reference must be reported as a tag.
func TestPushHandler_Render_untaggedRawReference(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := NewPushHandler(buf, "{{len .referenceAsTags}}").(*PushHandler)

	opts := &option.Target{
		RawReference: "localhost:5000/test",
		Path:         "localhost:5000/test",
	}
	if err := handler.OnCopied(opts, testDescriptor); err != nil {
		t.Fatalf("OnCopied() error = %v, want nil", err)
	}
	if err := handler.Render(); err != nil {
		t.Fatalf("Render() error = %v, want nil", err)
	}
	if got, want := buf.String(), "0"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}
