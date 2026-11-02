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

package credential

import (
	"github.com/oras-project/oras-go/v3/registry/remote/credentials"
)

// NewStore generates a store based on the passed-in config file paths.
func NewStore(configPaths ...string) (credentials.Store, error) {
	opts := credentials.StoreOptions{
		AllowPlaintextPut: true,
		// oras-go v3 detects the platform-default native credentials store by
		// default. Opt out to keep the v2 behavior of writing to the plain-text
		// config file, so that `oras login` does not silently start storing
		// credentials in the OS keychain.
		IgnoreDefaultNativeStore: true,
	}
	if len(configPaths) == 0 {
		// use default docker config file path
		return credentials.NewStoreFromDocker(opts)
	}

	var stores []credentials.Store
	for _, config := range configPaths {
		store, err := credentials.NewStore(config, opts)
		if err != nil {
			return nil, err
		}
		stores = append(stores, store)
	}
	return credentials.NewStoreWithFallbacks(stores[0], stores[1:]...), nil
}
