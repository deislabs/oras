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
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras/cmd/oras/internal/output"
)

type authCacheContextKey struct{}

// WithSharedAuthCache allows multiple command executions using the returned
// context to reuse authentication tokens when credentials are unchanged.
// Use a new context when credentials change or cache isolation is required.
func WithSharedAuthCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, authCacheContextKey{}, newAuthCacheStore())
}

// authCacheStore holds auth caches keyed by credential identity. A fresh store
// is used for each invocation unless the caller explicitly opts into reuse.
type authCacheStore struct {
	mu     sync.Mutex
	caches map[credentialIdentity]auth.Cache
}

// newAuthCacheStore creates an empty auth cache store.
func newAuthCacheStore() *authCacheStore {
	return &authCacheStore{caches: map[credentialIdentity]auth.Cache{}}
}

// cache returns the auth cache for the given identity, creating it if needed.
func (s *authCacheStore) cache(identity credentialIdentity) auth.Cache {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.caches[identity]; ok {
		return c
	}
	c := auth.NewCache()
	s.caches[identity] = c
	return c
}

// Common option struct.
type Common struct {
	Printer    *output.Printer
	Debug      bool
	authCaches *authCacheStore
}

// ApplyFlags applies flags to a command flag set.
func (opts *Common) ApplyFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&opts.Debug, "debug", "d", false, "output debug logs (implies --no-tty)")
}

// Parse gets target options from user input.
func (opts *Common) Parse(cmd *cobra.Command) error {
	opts.Printer = output.NewPrinter(cmd.OutOrStdout(), cmd.OutOrStderr())
	// Reset on each execution, including re-executions of the same command.
	opts.authCaches = newAuthCacheStore()
	if ctx := cmd.Context(); ctx != nil {
		if shared, ok := ctx.Value(authCacheContextKey{}).(*authCacheStore); ok {
			opts.authCaches = shared
		}
	}
	return nil
}
