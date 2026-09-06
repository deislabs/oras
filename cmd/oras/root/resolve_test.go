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
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/memory"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
)

func Test_doResolve_callsInjectedHandler(t *testing.T) {
	ctx := context.Background()
	target := memory.New()
	desc, err := oras.PackManifest(ctx, target, oras.PackManifestVersion1_1, "application/example", oras.PackManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Tag(ctx, desc, "source"); err != nil {
		t.Fatal(err)
	}
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingResolveHandler{Recorder: recorder, Err: errors.New("resolve output failed")}
	err = doResolve(ctx, target, "source", oras.DefaultResolveOptions, handler)
	if !errors.Is(err, handler.Err) {
		t.Fatalf("doResolve() error = %v, want %v", err, handler.Err)
	}
	if got, want := recorder.Snapshot(), []string{"OnResolved"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
	if desc.Digest == "" {
		t.Fatal("PackManifest returned empty digest")
	}
}

func Test_doResolve_resolveError(t *testing.T) {
	err := doResolve(context.Background(), memory.New(), "missing", oras.DefaultResolveOptions, &displaytest.RecordingResolveHandler{Recorder: &displaytest.CallRecorder{}})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve digest") {
		t.Fatalf("doResolve() error = %v, want wrapped resolve error", err)
	}
}
