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

package blob

import (
	"context"
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/errdef"

	"oras.land/oras/cmd/oras/internal/display/displaytest"
	"oras.land/oras/cmd/oras/internal/option"
)

type fakeBlobDeleter struct {
	resolveErr error
	deleteErr  error
	deleted    bool
}

func (f *fakeBlobDeleter) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{
		Digest: "sha256:1234",
	}, f.resolveErr
}

func (f *fakeBlobDeleter) Delete(context.Context, ocispec.Descriptor) error {
	f.deleted = true
	return f.deleteErr
}

var _ option.ResolvableDeleter = (*fakeBlobDeleter)(nil)

func Test_doDeleteBlob_blobMissing(t *testing.T) {
	opts := &deleteBlobOptions{}
	opts.Reference = "sha256:missing"
	opts.RawReference = "example.com/repo@sha256:missing"
	opts.Force = true

	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingBlobDeleteHandler{Recorder: recorder}
	deleter := &fakeBlobDeleter{resolveErr: errdef.ErrNotFound}

	err := doDeleteBlob(
		context.Background(),
		&cobra.Command{},
		opts,
		deleter,
		handler,
	)
	if err != nil {
		t.Fatalf("doDeleteBlob() error = %v, want nil", err)
	}
	if got, want := recorder.Snapshot(), []string{"OnBlobMissing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_deleteBlob_invalidTargetType(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	opts := &deleteBlobOptions{}
	opts.Type = "invalid"

	err := deleteBlob(cmd, opts)
	if err == nil {
		t.Fatal("deleteBlob() error = nil, want unknown target type error")
	}
	if got, want := err.Error(), `unknown target type: "invalid"`; got != want {
		t.Fatalf("deleteBlob() error = %q, want %q", got, want)
	}
}

func Test_doDeleteBlob_deleted(t *testing.T) {
	opts := &deleteBlobOptions{}
	opts.Reference = "sha256:1234"
	opts.RawReference = "example.com/repo@sha256:1234"
	opts.Force = true

	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingBlobDeleteHandler{Recorder: recorder}
	deleter := &fakeBlobDeleter{}

	err := doDeleteBlob(
		context.Background(),
		&cobra.Command{},
		opts,
		deleter,
		handler,
	)
	if err != nil {
		t.Fatalf("doDeleteBlob() error = %v, want nil", err)
	}
	if !deleter.deleted {
		t.Fatal("Delete() was not called")
	}
	if got, want := recorder.Snapshot(), []string{"OnBlobDeleted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
