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
)

func TestWithScopeHint(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		// host under which the auth client looks the scopes up
		wantHost   string
		wantScopes []string
	}{
		{
			// docker.io resolves to registry-1.docker.io, so the hint must be
			// keyed by the request host rather than the registry name.
			name:       "docker hub",
			ref:        "docker.io/library/alpine",
			wantHost:   "registry-1.docker.io",
			wantScopes: []string{"repository:library/alpine:pull,push"},
		},
		{
			name:       "other registry",
			ref:        "registry.example.com/team/app",
			wantHost:   "registry.example.com",
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
			if got := auth.GetScopesForHost(ctx, tt.wantHost); !reflect.DeepEqual(got, tt.wantScopes) {
				t.Errorf("GetScopesForHost(%q) = %v, want %v", tt.wantHost, got, tt.wantScopes)
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
