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
	"os"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/content/oci"
	"oras.land/oras/internal/cache"
)

type Cache struct {
	Root string
}

// CachedTarget gets the target storage with caching if cache root is specified.
func (opts *Cache) CachedTarget(src oras.ReadOnlyTarget) (oras.ReadOnlyTarget, error) {
	opts.Root = os.Getenv("ORAS_CACHE")
	if opts.Root != "" {
		ociStore, err := oci.New(opts.Root)
		if err != nil {
			return nil, err
		}
		return cache.New(src, ociStore), nil
	}
	return src, nil
}
