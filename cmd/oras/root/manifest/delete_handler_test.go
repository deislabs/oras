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
	"context"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/spf13/cobra"

	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/option"
)

type testManifestDeleteDeleter struct {
	resolveErr error
	deleteErr  error
	deleted    bool
}

func (f *testManifestDeleteDeleter) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{
		Digest: "sha256:1234",
	}, f.resolveErr
}

func (f *testManifestDeleteDeleter) Delete(context.Context, ocispec.Descriptor) error {
	f.deleted = true
	return f.deleteErr
}

var _ option.ResolvableDeleter = (*testManifestDeleteDeleter)(nil)

func Test_doDeleteManifest_manifestMissing(t *testing.T) {
	opts := &deleteOptions{}
	opts.Reference = "sha256:missing"
	opts.RawReference = "example.com/repo@sha256:missing"
	opts.Force = true

	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingManifestDeleteHandler{Recorder: recorder}
	deleter := &testManifestDeleteDeleter{resolveErr: errdef.ErrNotFound}

	err := doDeleteManifest(
		context.Background(),
		&cobra.Command{},
		opts,
		deleter,
		handler,
	)
	if err != nil {
		t.Fatalf("doDeleteManifest() error = %v, want nil", err)
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestMissing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_deleteManifest_invalidTargetType(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	opts := &deleteOptions{}
	opts.Type = "invalid"

	err := deleteManifest(cmd, opts)
	if err == nil {
		t.Fatal("deleteManifest() error = nil, want unknown target type error")
	}
	if got, want := err.Error(), `unknown target type: "invalid"`; got != want {
		t.Fatalf("deleteManifest() error = %q, want %q", got, want)
	}
}

func Test_doDeleteManifest_deleted(t *testing.T) {
	opts := &deleteOptions{}
	opts.Reference = "sha256:1234"
	opts.RawReference = "example.com/repo@sha256:1234"
	opts.Force = true

	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingManifestDeleteHandler{Recorder: recorder}
	deleter := &testManifestDeleteDeleter{}

	err := doDeleteManifest(
		context.Background(),
		&cobra.Command{},
		opts,
		deleter,
		handler,
	)
	if err != nil {
		t.Fatalf("doDeleteManifest() error = %v, want nil", err)
	}
	if !deleter.deleted {
		t.Fatal("Delete() was not called")
	}
	if got, want := recorder.Snapshot(), []string{"OnManifestDeleted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
