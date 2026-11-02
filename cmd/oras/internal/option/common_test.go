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
	"context"
	"testing"

	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestCommonParse_CreatesFreshAuthCaches(t *testing.T) {
	opts := Common{}
	cmd := &cobra.Command{}
	if err := opts.Parse(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	first := opts.authCaches
	if first == nil {
		t.Fatal("expected an auth cache store after Parse")
	}

	if err := opts.Parse(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.authCaches == first {
		t.Fatal("expected a fresh auth cache store for a new invocation")
	}
}

func TestCommonParse_ExplicitSharedAuthCache(t *testing.T) {
	ctx := WithSharedAuthCache(context.Background())
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	first := Common{}
	if err := first.Parse(cmd); err != nil {
		t.Fatal(err)
	}
	if first.authCaches == nil {
		t.Fatal("expected an auth cache store")
	}
	const registry, scope = "registry.example.com", "repository:test:pull"
	identity := newCredentialIdentity(auth.Credential{Username: "user", Password: "password"}, nil)
	if _, err := first.authCaches.cache(identity).Set(ctx, registry, auth.SchemeBearer, scope, func(context.Context) (string, error) {
		return "cached-token", nil
	}); err != nil {
		t.Fatal(err)
	}

	// Re-execution of the same command uses the caller's shared store.
	firstStore := first.authCaches
	if err := first.Parse(cmd); err != nil {
		t.Fatal(err)
	}
	if first.authCaches != firstStore {
		t.Fatal("expected injected cache to survive command re-execution")
	}
	second := Common{}
	otherCmd := &cobra.Command{}
	otherCmd.SetContext(ctx)
	if err := second.Parse(otherCmd); err != nil {
		t.Fatal(err)
	}
	if first.authCaches != second.authCaches {
		t.Fatal("expected shared cache across command instances")
	}
	got, err := second.authCaches.cache(identity).GetToken(ctx, registry, auth.SchemeBearer, scope)
	if err != nil || got != "cached-token" {
		t.Fatalf("expected cached token across executions, got %q: %v", got, err)
	}

	// A separately configured context must not reuse cached tokens.
	isolated := Common{}
	otherCmd.SetContext(WithSharedAuthCache(context.Background()))
	if err := isolated.Parse(otherCmd); err != nil {
		t.Fatal(err)
	}
	if isolated.authCaches == first.authCaches {
		t.Fatal("expected a fresh cache for a different context")
	}
	if _, err := isolated.authCaches.cache(identity).GetToken(ctx, registry, auth.SchemeBearer, scope); err == nil {
		t.Fatal("expected cached token to be isolated from a different context")
	}
}
