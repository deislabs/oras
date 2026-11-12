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
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func Test_deleteCmd_confirmationFromCmdStdin(t *testing.T) {
	manifestBytes := []byte("{}")
	testDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifestBytes))
	const tagPath = "/v2/test/manifests/v1"
	digestPath := "/v2/test/manifests/" + testDigest

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
				switch {
				case r.Method == http.MethodHead && r.URL.Path == tagPath:
					w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
					w.Header().Set("Docker-Content-Digest", testDigest)
					w.Header().Set("Content-Length", strconv.Itoa(len(manifestBytes)))
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet && r.URL.Path == digestPath:
					w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
					w.Header().Set("Docker-Content-Digest", testDigest)
					w.Header().Set("Content-Length", strconv.Itoa(len(manifestBytes)))
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(manifestBytes)
				case r.Method == http.MethodDelete && r.URL.Path == digestPath:
					deleted = true
					w.WriteHeader(http.StatusAccepted)
				default:
					w.WriteHeader(http.StatusNotFound)
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
				fmt.Sprintf("%s/test:v1", uri.Host),
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
