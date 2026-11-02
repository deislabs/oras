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

package registryutil

import (
	"context"
	"reflect"
	"testing"

	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"
)

func TestWithScopeHint(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		// resource, as derived from the request, under which the auth client
		// looks the scopes up
		lookup     properties.Resource
		wantScopes []string
	}{
		{
			// docker.io resolves to registry-1.docker.io, so the hint must be
			// found when looked up by the request host.
			name:       "docker hub",
			ref:        "docker.io/library/alpine",
			lookup:     properties.Resource{Registry: "registry-1.docker.io", Path: "library/alpine"},
			wantScopes: []string{"repository:library/alpine:pull,push"},
		},
		{
			name:       "other registry",
			ref:        "registry.example.com/team/app",
			lookup:     properties.Resource{Registry: "registry.example.com", Path: "team/app"},
			wantScopes: []string{"repository:team/app:pull,push"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := remote.NewRepository(tt.ref)
			if err != nil {
				t.Fatalf("NewRepository() error = %v", err)
			}
			ctx := WithScopeHint(context.Background(), repo, auth.ActionPull, auth.ActionPush)
			var got []string
			for _, scope := range auth.GetScopesForResource(ctx, tt.lookup) {
				got = append(got, scope.String())
			}
			if !reflect.DeepEqual(got, tt.wantScopes) {
				t.Errorf("GetScopesForResource(%v) = %v, want %v", tt.lookup, got, tt.wantScopes)
			}
		})
	}
}

func TestWithScopeHint_notRemote(t *testing.T) {
	ctx := context.Background()
	if got := WithScopeHint(ctx, "not a repository", auth.ActionPull); got != ctx {
		t.Error("WithScopeHint() should return the context unchanged for non-remote targets")
	}
}
