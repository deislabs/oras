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

package repository

import "testing"

func Test_ParseRepoPath(t *testing.T) {
	type args struct {
		rawReference string
	}
	tests := []struct {
		name          string
		args          args
		wantHostname  string
		wantNamespace string
		wantErr       bool
	}{
		{
			name:          "hostname only",
			args:          args{"testregistry.example.io"},
			wantErr:       false,
			wantHostname:  "testregistry.example.io",
			wantNamespace: "",
		},
		{
			name:          "hostname with trailing slash",
			args:          args{"testregistry.example.io/"},
			wantErr:       false,
			wantHostname:  "testregistry.example.io",
			wantNamespace: "",
		},
		{
			name:          "hostname and repo",
			args:          args{"testregistry.example.io/showcase"},
			wantErr:       false,
			wantHostname:  "testregistry.example.io",
			wantNamespace: "showcase/",
		},
		{
			name:          "hostname and repo in a sub-namespace",
			args:          args{"testregistry.example.io/showcase/beta"},
			wantErr:       false,
			wantHostname:  "testregistry.example.io",
			wantNamespace: "showcase/beta/",
		},
		{
			name:          "hostname and repo in a sub-namespace with trailing slash",
			args:          args{"testregistry.example.io/showcase/beta/"},
			wantErr:       false,
			wantHostname:  "testregistry.example.io",
			wantNamespace: "showcase/beta/",
		},
		{
			name:          "error when a tag is provided",
			args:          args{"testregistry.example.io/showcase:latest"},
			wantErr:       true,
			wantHostname:  "",
			wantNamespace: "",
		},
		{
			name:          "error when a digest is provided",
			args:          args{"testregistry.example.io/showcase:sha256:2e0e0fe1fb3edbcdddad941c90d2b51e25a6bcd593e82545441a216de7bfa834"},
			wantErr:       true,
			wantHostname:  "",
			wantNamespace: "",
		},
		{
			name:          "error when a malformed path is provided",
			args:          args{"testregistry.example.io///showcase/"},
			wantErr:       true,
			wantHostname:  "",
			wantNamespace: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHostname, gotNamespace, err := ParseRemoteRepository(tt.args.rawReference)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRepoPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotHostname != tt.wantHostname {
				t.Errorf("parseRepoPath() gotHostname = %v, want %v", gotHostname, tt.wantHostname)
			}
			if gotNamespace != tt.wantNamespace {
				t.Errorf("parseRepoPath() gotNamespace = %v, want %v", gotNamespace, tt.wantNamespace)
			}
		})
	}
}

func TestParseReference(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"localhost:5000/repo:tag", false},
		{"http://127.0.0.1:1/repo:tag", true},
		{"https://localhost:5000/repo", true},
		{"oci://localhost:5000/repo:tag", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			_, err := ParseReference(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReference(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestParseRemoteRepository_scheme(t *testing.T) {
	if _, _, err := ParseRemoteRepository("http://localhost:5000/ns"); err == nil {
		t.Error("expect ParseRemoteRepository to reject a URL scheme")
	}
}
