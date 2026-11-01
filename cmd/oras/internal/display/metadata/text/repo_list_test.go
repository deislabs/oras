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

package text

import (
	"bytes"
	"testing"
)

func TestRepoListHandler(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		repos     []string
		want      string
	}{
		{
			name:      "with namespace",
			namespace: "example/",
			repos:     []string{"example/app", "example/library"},
			want:      "app\nlibrary\n",
		},
		{
			name:  "without namespace",
			repos: []string{"example/app"},
			want:  "example/app\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			handler := NewRepoListHandler(&out, tt.namespace)

			for _, repo := range tt.repos {
				if err := handler.OnRepositoryListed(repo); err != nil {
					t.Fatalf("OnRepositoryListed(%q): %v", repo, err)
				}
			}

			if got := out.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}

			if err := handler.Render(); err != nil {
				t.Fatalf("Render(): %v", err)
			}
		})
	}
}
