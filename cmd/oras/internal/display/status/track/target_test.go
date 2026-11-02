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

package track

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"github.com/oras-project/oras-go/v3/content/oci"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"oras.land/oras/internal/progress"
	"oras.land/oras/internal/testutils"
)

type testReferenceGraphTarget struct {
	oras.GraphTarget
}

type testReferenceMounterGraphTarget struct {
	*testReferenceGraphTarget
	registry.Mounter
}

func (t *testReferenceGraphTarget) PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	err := t.Push(ctx, expected, content)
	if err != nil {
		return err
	}
	return t.Tag(ctx, expected, reference)
}

func Test_referenceGraphTarget_PushReference(t *testing.T) {
	// prepare
	reader, device, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = device.Close() }()
	src := memory.New()
	content := []byte("test")
	r := bytes.NewReader(content)
	desc := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
	}
	// test
	tag := "tagged"
	donePrompt := "done"
	prompt := map[progress.State]string{
		progress.StateTransmitted: donePrompt,
	}
	target, err := NewTarget(&testReferenceGraphTarget{src}, prompt, device)
	if err != nil {
		t.Fatal(err)
	}
	if rgt, ok := target.(*referenceGraphTarget); ok {
		if err := rgt.PushReference(context.Background(), desc, r, tag); err != nil {
			t.Fatal(err)
		}
		if err := rgt.manager.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("not testing based on a referenceGraphTarget")
	}
	// validate
	if err = testutils.MatchPipe(reader, device, donePrompt, desc.MediaType, "100.00%", desc.Digest.String()); err != nil {
		t.Fatal(err)
	}
}

func TestNewTarget_mountCapabilityMatchesUnderlyingTarget(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		underlying    oras.GraphTarget
		wantMount     bool
		wantReference bool
	}{
		{name: "memory", underlying: memory.New(), wantMount: false},
		{name: "OCI layout", underlying: store, wantMount: false},
		{name: "remote", underlying: &remote.Repository{}, wantMount: true, wantReference: true},
		{name: "reference", underlying: &testReferenceGraphTarget{memory.New()}, wantReference: true},
		{name: "reference and mount", underlying: &testReferenceMounterGraphTarget{
			testReferenceGraphTarget: &testReferenceGraphTarget{memory.New()},
			Mounter:                  &remote.Repository{},
		}, wantMount: true, wantReference: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, device, err := testutils.NewPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = device.Close() }()
			tracked, err := NewTarget(tc.underlying, nil, device)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tracked.Close() }()
			_, mounted := tracked.(registry.Mounter)
			if mounted != tc.wantMount {
				t.Errorf("registry.Mounter supported = %v, want %v", mounted, tc.wantMount)
			}
			_, reference := tracked.(registry.ReferencePusher)
			if reference != tc.wantReference {
				t.Errorf("registry.ReferencePusher supported = %v, want %v", reference, tc.wantReference)
			}
		})
	}
}

func Test_graphTarget_Push_alreadyExists(t *testing.T) {
	// prepare
	reader, device, err := testutils.NewPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = device.Close() }()
	src := memory.New()
	content := []byte("test")
	r := bytes.NewReader(content)
	desc := ocispec.Descriptor{
		MediaType: "application/octet-stream",
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
	}
	if err := src.Push(context.Background(), desc, r); err != nil {
		t.Fatal("Failed to prepare test environment:", err)
	}
	// test
	donePrompt := "done"
	prompt := map[progress.State]string{
		progress.StateTransmitted: donePrompt,
	}
	target, err := NewTarget(src, prompt, device)
	if err != nil {
		t.Fatal(err)
	}
	if gt, ok := target.(*graphTarget); ok {
		if err := gt.Push(context.Background(), desc, r); !errors.Is(err, errdef.ErrAlreadyExists) {
			t.Fatal("Expected ErrAlreadyExists, got:", err)
		}
		if err := gt.manager.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("not testing based on a referenceGraphTarget")
	}
	// validate
	if err = testutils.MatchPipe(reader, device, donePrompt, desc.MediaType, "100.00%", desc.Digest.String()); err != nil {
		t.Fatal(err)
	}
}
