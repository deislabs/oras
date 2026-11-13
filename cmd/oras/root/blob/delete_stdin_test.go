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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func Test_deleteCmd_confirmationFromCmdStdin(t *testing.T) {
	const testDigest = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	const blobPath = "/v2/test/blobs/" + testDigest

	tests := []struct {
		name        string
		stdin       string
		wantDeleted bool
	}{
		{
			name:        "confirmed via cmd stdin",
			stdin:       "y\n",
			wantDeleted: true,
		},
		{
			name:        "cancelled via cmd stdin",
			stdin:       "n\n",
			wantDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted := false
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != blobPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				switch r.Method {
				case http.MethodHead:
					w.Header().Set("Docker-Content-Digest", testDigest)
					w.Header().Set("Content-Length", "4")
					w.WriteHeader(http.StatusOK)
				case http.MethodDelete:
					deleted = true
					w.WriteHeader(http.StatusAccepted)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer ts.Close()

			uri, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}

			cmd := deleteCmd()
			cmd.SetIn(strings.NewReader(tt.stdin))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{
				"--plain-http",
				"--registry-config", filepath.Join(t.TempDir(), "config.json"),
				fmt.Sprintf("%s/test@%s", uri.Host, testDigest),
			})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("deleteCmd() error = %v", err)
			}
			if deleted != tt.wantDeleted {
				t.Errorf("deleted = %v, want %v", deleted, tt.wantDeleted)
			}
		})
	}
}
