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

package repo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras/cmd/oras/internal/display/displaytest"
)

func Test_emitRepositories_filtersAndStopsOnError(t *testing.T) {
	wantedErr := errors.New("listing failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingRepoListHandler{Recorder: recorder, RepositoryListedErr: wantedErr}
	err := emitRepositories([]string{"other/b", "example/a", "example/c"}, "example/", handler)
	if !errors.Is(err, wantedErr) {
		t.Fatalf("emitRepositories() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnRepositoryListed:example/a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func newCatalogTestRegistry(t *testing.T) *remote.Registry {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/_catalog" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"repositories":["example/a","other/b"]}`))
	}))
	t.Cleanup(server.Close)
	reg, err := remote.NewRegistry(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	reg.PlainHTTP = true
	return reg
}

func Test_doListRepository_handlerCalls(t *testing.T) {
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingRepoListHandler{Recorder: recorder}
	if err := doListRepository(context.Background(), newCatalogTestRegistry(t), "", "example/", handler); err != nil {
		t.Fatal(err)
	}
	if got, want := recorder.Snapshot(), []string{"OnRepositoryListed:example/a", "Render"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}

func Test_doListRepository_handlerErrorSkipsRender(t *testing.T) {
	wantedErr := errors.New("listing failed")
	recorder := &displaytest.CallRecorder{}
	handler := &displaytest.RecordingRepoListHandler{Recorder: recorder, RepositoryListedErr: wantedErr}
	if err := doListRepository(context.Background(), newCatalogTestRegistry(t), "", "", handler); !errors.Is(err, wantedErr) {
		t.Fatalf("doListRepository() error = %v, want %v", err, wantedErr)
	}
	if got, want := recorder.Snapshot(), []string{"OnRepositoryListed:example/a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls = %v, want %v", got, want)
	}
}
