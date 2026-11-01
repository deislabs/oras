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

func TestRepoTagsHandler(t *testing.T) {
	var out bytes.Buffer
	handler := NewRepoTagsHandler(&out)

	for _, tag := range []string{"v1", "v2"} {
		if err := handler.OnTagListed(tag); err != nil {
			t.Fatalf("OnTagListed(%q): %v", tag, err)
		}
	}

	if got, want := out.String(), "v1\nv2\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}

	if err := handler.Render(); err != nil {
		t.Fatalf("Render(): %v", err)
	}
}
