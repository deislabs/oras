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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func Test_loginCmd_interactiveStdin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	uri, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	tests := []struct {
		name         string
		stdin        string
		wantPrompt   string
		wantAuthText string
	}{
		{
			name:         "username and password from cmd stdin",
			stdin:        "alice\ns3cr3t\n",
			wantPrompt:   "Username: Password: ",
			wantAuthText: "alice:s3cr3t",
		},
		{
			name:       "identity token from cmd stdin when username is empty",
			stdin:      "\nmy-token\n",
			wantPrompt: "Username: Token: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			var outBuf bytes.Buffer
			cmd := loginCmd()
			cmd.SetIn(strings.NewReader(tt.stdin))
			cmd.SetOut(&outBuf)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--plain-http", "--registry-config", configPath, uri.Host})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("loginCmd() error = %v", err)
			}
			if !strings.Contains(outBuf.String(), tt.wantPrompt) {
				t.Errorf("output = %q, want substring %q", outBuf.String(), tt.wantPrompt)
			}

			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("os.ReadFile() error = %v", err)
			}
			var cfg struct {
				Auths map[string]struct {
					Auth          string `json:"auth"`
					IdentityToken string `json:"identitytoken"`
				} `json:"auths"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			entry, ok := cfg.Auths[uri.Host]
			if !ok {
				t.Fatalf("config missing entry for %q", uri.Host)
			}
			if tt.wantAuthText != "" {
				decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
				if err != nil {
					t.Fatalf("base64.StdEncoding.DecodeString() error = %v", err)
				}
				if string(decoded) != tt.wantAuthText {
					t.Errorf("decoded Auth = %q, want %q", string(decoded), tt.wantAuthText)
				}
			} else if entry.IdentityToken != "my-token" {
				t.Errorf("IdentityToken = %q, want %q", entry.IdentityToken, "my-token")
			}
		})
	}
}

func Test_loginCmd_interactiveStdinError(t *testing.T) {
	wantErr := errors.New("stdin read failed")
	for _, args := range [][]string{
		{"localhost:5000"},
		{"-u", "alice", "localhost:5000"},
	} {
		cmd := loginCmd()
		cmd.SetIn(iotest.ErrReader(wantErr))
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--registry-config", filepath.Join(t.TempDir(), "config.json")}, args...))
		if err := cmd.Execute(); !errors.Is(err, wantErr) {
			t.Errorf("loginCmd(%v) error = %v, want %v", args, err, wantErr)
		}
	}
}
