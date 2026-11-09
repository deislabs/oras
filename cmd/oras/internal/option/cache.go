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
	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/cache"
)

type Cache struct {
	Root string
}

// CachedTarget gets the target storage with caching if the ORAS_CACHE
// environment variable sets a cache root. The cache is backed by oras-go's
// content/cache, which keeps the source's ReferenceFetcher, ReferenceParser
// and ReferrerLister interfaces and stores content with the process-safe
// oci.NewStorage.
func (opts *Cache) CachedTarget(src oras.ReadOnlyTarget) (oras.ReadOnlyTarget, error) {
	c := cache.NewFromEnv()
	if c == nil {
		opts.Root = ""
		return src, nil
	}
	opts.Root = c.Root
	return c.ReadOnlyTarget(src)
}
