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

import "testing"

func TestValidateExtraTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr bool
	}{
		{
			name: "no extra tags",
		},
		{
			name: "valid tags",
			tags: []string{"v2", "latest"},
		},
		{
			name:    "empty tag",
			tags:    []string{""},
			wantErr: true,
		},
		{
			name:    "empty tag between valid tags",
			tags:    []string{"v2", "", "v3"},
			wantErr: true,
		},
		{
			name:    "invalid tag",
			tags:    []string{"invalid@@tag"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtraTags(tt.tags)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateExtraTags(%q) error = %v, wantErr %t",
					tt.tags, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOCILayoutPathReference(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{
			name: "valid repository and tag",
			ref:  "example.com/repo:v1",
		},
		{
			name:    "invalid repository containing space",
			ref:     "example.com/re po:v1",
			wantErr: true,
		},
		{
			name:    "invalid tag containing space",
			ref:     "example.com/repo:BAD TAG",
			wantErr: true,
		},
		{
			name:    "invalid tag character",
			ref:     "example.com/repo:bad!tag",
			wantErr: true,
		},
		{
			name: "valid digest",
			ref:  "example.com/repo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name:    "invalid digest",
			ref:     "example.com/repo@sha256:invalid",
			wantErr: true,
		},
		{
			name: "valid standalone tag",
			ref:  "latest",
		},
		{
			name:    "empty reference",
			ref:     "",
			wantErr: true,
		},
		{
			name:    "empty repository",
			ref:     ":v1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOCILayoutPathReference(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOCILayoutPathReference(%q) error = %v, wantErr %t",
					tt.ref, err, tt.wantErr)
			}
		})
	}
}
