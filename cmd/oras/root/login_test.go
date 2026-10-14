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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_loginAndLogout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("Www-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	uri, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")

	login := loginCmd()
	login.SetContext(context.Background())
	login.SetArgs([]string{"--plain-http", "--registry-config", configPath, "-u", "user", "-p", "secret", uri.Host})
	if err := login.Execute(); err != nil {
		t.Fatalf("login error = %v", err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if !strings.Contains(string(content), uri.Host) {
		t.Fatalf("config %s does not contain credential for %q", content, uri.Host)
	}

	logout := logoutCmd()
	logout.SetContext(context.Background())
	logout.SetArgs([]string{"--registry-config", configPath, uri.Host})
	if err := logout.Execute(); err != nil {
		t.Fatalf("logout error = %v", err)
	}
	if content, err = os.ReadFile(configPath); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if strings.Contains(string(content), uri.Host) {
		t.Fatalf("config %s still contains credential for %q", content, uri.Host)
	}
}

func Test_runLogin_invalidRegistry(t *testing.T) {
	cmd := loginCmd()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--registry-config", filepath.Join(t.TempDir(), "config.json"), "-u", "user", "-p", "secret", "invalid registry"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("login error = nil, want error")
	}
}
