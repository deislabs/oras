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

package option

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestRecursive_ApplyFlags(t *testing.T) {
	var opts Recursive
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opts.ApplyFlags(fs)

	// Check that all flags are registered
	if fs.Lookup("recursive") == nil {
		t.Error("expected 'recursive' flag to be registered")
	}
	if fs.Lookup("max-blobs-per-manifest") == nil {
		t.Error("expected 'max-blobs-per-manifest' flag to be registered")
	}
	if fs.Lookup("preserve-empty-dirs") == nil {
		t.Error("expected 'preserve-empty-dirs' flag to be registered")
	}
	if fs.Lookup("follow-symlinks") == nil {
		t.Error("expected 'follow-symlinks' flag to be registered")
	}
}

func TestRecursive_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    Recursive
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid defaults",
			opts: Recursive{
				Recursive:           false,
				MaxBlobsPerManifest: DefaultMaxBlobsPerManifest,
			},
			wantErr: false,
		},
		{
			name: "valid recursive enabled",
			opts: Recursive{
				Recursive:           true,
				MaxBlobsPerManifest: 500,
			},
			wantErr: false,
		},
		{
			name: "invalid max blobs less than 1",
			opts: Recursive{
				Recursive:           true,
				MaxBlobsPerManifest: 0,
			},
			wantErr: true,
			errMsg:  "--max-blobs-per-manifest must be at least 1",
		},
		{
			name: "invalid max blobs exceeds limit",
			opts: Recursive{
				Recursive:           true,
				MaxBlobsPerManifest: 20000,
			},
			wantErr: true,
			errMsg:  "--max-blobs-per-manifest exceeds maximum allowed value of 10000",
		},
		{
			name: "preserve-empty-dirs without recursive",
			opts: Recursive{
				Recursive:           false,
				MaxBlobsPerManifest: DefaultMaxBlobsPerManifest,
				PreserveEmptyDirs:   true,
			},
			wantErr: true,
			errMsg:  "--preserve-empty-dirs requires --recursive",
		},
		{
			name: "follow-symlinks without recursive",
			opts: Recursive{
				Recursive:           false,
				MaxBlobsPerManifest: DefaultMaxBlobsPerManifest,
				FollowSymlinks:      true,
			},
			wantErr: true,
			errMsg:  "--follow-symlinks requires --recursive",
		},
		{
			name: "all options with recursive enabled",
			opts: Recursive{
				Recursive:           true,
				MaxBlobsPerManifest: 1000,
				PreserveEmptyDirs:   true,
				FollowSymlinks:      true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}
